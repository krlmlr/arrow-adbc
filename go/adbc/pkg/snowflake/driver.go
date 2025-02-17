// Code generated by _tmpl/driver.go.tmpl. DO NOT EDIT.

// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

//go:build driverlib

package main

// #cgo CXXFLAGS: -std=c++11
// #include "../../drivermgr/adbc.h"
// #include "utils.h"
// #include <stdint.h>
// #include <string.h>
//
// typedef const char cchar_t;
// typedef const uint8_t cuint8_t;
//
// void releasePartitions(struct AdbcPartitions* partitions);
//
import "C"
import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/cgo"
	"unsafe"

	"github.com/apache/arrow-adbc/go/adbc"
	"github.com/apache/arrow-adbc/go/adbc/driver/snowflake"
	"github.com/apache/arrow/go/v12/arrow/array"
	"github.com/apache/arrow/go/v12/arrow/cdata"
	"github.com/apache/arrow/go/v12/arrow/memory/mallocator"
)

var drv = snowflake.Driver{Alloc: mallocator.NewMallocator()}

const errPrefix = "[Snowflake] "

func setErr(err *C.struct_AdbcError, format string, vals ...interface{}) {
	if err == nil {
		return
	}

	if err.release != nil {
		C.SnowflakeerrRelease(err)
	}

	msg := errPrefix + fmt.Sprintf(format, vals...)
	err.message = C.CString(msg)
	err.release = (*[0]byte)(C.Snowflake_release_error)
}

func errToAdbcErr(adbcerr *C.struct_AdbcError, err error) adbc.Status {
	if adbcerr == nil || err == nil {
		return adbc.StatusOK
	}

	var adbcError adbc.Error
	if errors.As(err, &adbcError) {
		setErr(adbcerr, adbcError.Msg)
		return adbcError.Code
	}

	setErr(adbcerr, err.Error())
	return adbc.StatusUnknown
}

// Allocate a new cgo.Handle and store its address in a heap-allocated
// uintptr_t.  Experimentally, this was found to be necessary, else
// something (the Go runtime?) would corrupt (garbage-collect?) the
// handle.
func createHandle(hndl cgo.Handle) unsafe.Pointer {
	// uintptr_t* hptr = malloc(sizeof(uintptr_t));
	hptr := (*C.uintptr_t)(C.malloc(C.sizeof_uintptr_t))
	// *hptr = (uintptr)hndl;
	*hptr = C.uintptr_t(uintptr(hndl))
	return unsafe.Pointer(hptr)
}

func getFromHandle[T any](ptr unsafe.Pointer) *T {
	// uintptr_t* hptr = (uintptr_t*)ptr;
	hptr := (*C.uintptr_t)(ptr)
	return cgo.Handle((uintptr)(*hptr)).Value().(*T)
}

func checkDBAlloc(db *C.struct_AdbcDatabase, err *C.struct_AdbcError, fname string) bool {
	if db == nil {
		setErr(err, "%s: database not allocated", fname)
		return false
	}
	if db.private_data == nil {
		setErr(err, "%s: database not allocated", fname)
		return false
	}
	return true
}

func checkDBInit(db *C.struct_AdbcDatabase, err *C.struct_AdbcError, fname string) *cDatabase {
	if !checkDBAlloc(db, err, fname) {
		return nil
	}
	cdb := getFromHandle[cDatabase](db.private_data)
	if cdb.db == nil {
		setErr(err, "%s: database not initialized", fname)
		return nil
	}

	return cdb
}

type cDatabase struct {
	opts map[string]string
	db   adbc.Database
}

//export SnowflakeDatabaseNew
func SnowflakeDatabaseNew(db *C.struct_AdbcDatabase, err *C.struct_AdbcError) C.AdbcStatusCode {
	if db.private_data != nil {
		setErr(err, "AdbcDatabaseNew: database already allocated")
		return C.ADBC_STATUS_INVALID_STATE
	}
	dbobj := &cDatabase{opts: make(map[string]string)}
	hndl := cgo.NewHandle(dbobj)
	db.private_data = createHandle(hndl)
	return C.ADBC_STATUS_OK
}

//export SnowflakeDatabaseSetOption
func SnowflakeDatabaseSetOption(db *C.struct_AdbcDatabase, key, value *C.cchar_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	if !checkDBAlloc(db, err, "AdbcDatabaseSetOption") {
		return C.ADBC_STATUS_INVALID_STATE
	}
	cdb := getFromHandle[cDatabase](db.private_data)

	k, v := C.GoString(key), C.GoString(value)
	cdb.opts[k] = v

	return C.ADBC_STATUS_OK
}

//export SnowflakeDatabaseInit
func SnowflakeDatabaseInit(db *C.struct_AdbcDatabase, err *C.struct_AdbcError) C.AdbcStatusCode {
	if !checkDBAlloc(db, err, "AdbcDatabaseInit") {
		return C.ADBC_STATUS_INVALID_STATE
	}
	cdb := getFromHandle[cDatabase](db.private_data)

	if cdb.db != nil {
		setErr(err, "AdbcDatabaseInit: database already initialized")
		return C.ADBC_STATUS_INVALID_STATE
	}

	adb, aerr := drv.NewDatabase(cdb.opts)
	if aerr != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, aerr))
	}

	cdb.db = adb
	return C.ADBC_STATUS_OK
}

//export SnowflakeDatabaseRelease
func SnowflakeDatabaseRelease(db *C.struct_AdbcDatabase, err *C.struct_AdbcError) C.AdbcStatusCode {
	if !checkDBAlloc(db, err, "AdbcDatabaseRelease") {
		return C.ADBC_STATUS_INVALID_STATE
	}
	h := (*(*cgo.Handle)(db.private_data))

	cdb := h.Value().(*cDatabase)
	cdb.db = nil
	cdb.opts = nil
	C.free(unsafe.Pointer(db.private_data))
	db.private_data = nil
	h.Delete()
	// manually trigger GC for two reasons:
	//  1. ASAN expects the release callback to be called before
	//     the process ends, but GC is not deterministic. So by manually
	//     triggering the GC we ensure the release callback gets called.
	//  2. Creates deterministic GC behavior by all Release functions
	//     triggering a garbage collection
	runtime.GC()
	return C.ADBC_STATUS_OK
}

type cConn struct {
	cnxn adbc.Connection
}

func checkConnAlloc(cnxn *C.struct_AdbcConnection, err *C.struct_AdbcError, fname string) bool {
	if cnxn == nil {
		setErr(err, "%s: connection not allocated", fname)
		return false
	}
	if cnxn.private_data == nil {
		setErr(err, "%s: connection not allocated", fname)
		return false
	}
	return true
}

func checkConnInit(cnxn *C.struct_AdbcConnection, err *C.struct_AdbcError, fname string) *cConn {
	if !checkConnAlloc(cnxn, err, fname) {
		return nil
	}
	conn := getFromHandle[cConn](cnxn.private_data)
	if conn.cnxn == nil {
		setErr(err, "%s: connection not initialized", fname)
		return nil
	}

	return conn
}

//export SnowflakeConnectionNew
func SnowflakeConnectionNew(cnxn *C.struct_AdbcConnection, err *C.struct_AdbcError) C.AdbcStatusCode {
	if cnxn.private_data != nil {
		setErr(err, "AdbcConnectionNew: connection already allocated")
		return C.ADBC_STATUS_INVALID_STATE
	}

	hndl := cgo.NewHandle(&cConn{})
	cnxn.private_data = createHandle(hndl)
	return C.ADBC_STATUS_OK
}

//export SnowflakeConnectionSetOption
func SnowflakeConnectionSetOption(cnxn *C.struct_AdbcConnection, key, val *C.cchar_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	if !checkConnAlloc(cnxn, err, "AdbcConnectionSetOption") {
		return C.ADBC_STATUS_INVALID_STATE
	}
	conn := getFromHandle[cConn](cnxn.private_data)

	code := errToAdbcErr(err, conn.cnxn.(adbc.PostInitOptions).SetOption(C.GoString(key), C.GoString(val)))
	return C.AdbcStatusCode(code)
}

//export SnowflakeConnectionInit
func SnowflakeConnectionInit(cnxn *C.struct_AdbcConnection, db *C.struct_AdbcDatabase, err *C.struct_AdbcError) C.AdbcStatusCode {
	if !checkConnAlloc(cnxn, err, "AdbcConnectionInit") {
		return C.ADBC_STATUS_INVALID_STATE
	}

	conn := getFromHandle[cConn](cnxn.private_data)
	if conn.cnxn != nil {
		setErr(err, "AdbcConnectionInit: connection already initialized")
		return C.ADBC_STATUS_INVALID_STATE
	}
	cdb := checkDBInit(db, err, "AdbcConnectionInit")
	if cdb == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}
	c, e := cdb.db.Open(context.Background())
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}

	conn.cnxn = c
	return C.ADBC_STATUS_OK
}

//export SnowflakeConnectionRelease
func SnowflakeConnectionRelease(cnxn *C.struct_AdbcConnection, err *C.struct_AdbcError) C.AdbcStatusCode {
	if !checkConnAlloc(cnxn, err, "AdbcConnectionRelease") {
		return C.ADBC_STATUS_INVALID_STATE
	}
	h := (*(*cgo.Handle)(cnxn.private_data))

	conn := h.Value().(*cConn)
	defer func() {
		conn.cnxn = nil
		C.free(unsafe.Pointer(cnxn.private_data))
		cnxn.private_data = nil
		h.Delete()
		// manually trigger GC for two reasons:
		//  1. ASAN expects the release callback to be called before
		//     the process ends, but GC is not deterministic. So by manually
		//     triggering the GC we ensure the release callback gets called.
		//  2. Creates deterministic GC behavior by all Release functions
		//     triggering a garbage collection
		runtime.GC()
	}()
	if conn.cnxn == nil {
		return C.ADBC_STATUS_OK
	}
	return C.AdbcStatusCode(errToAdbcErr(err, conn.cnxn.Close()))
}

func fromCArr[T, CType any](ptr *CType, sz int) []T {
	if ptr == nil || sz == 0 {
		return nil
	}

	return unsafe.Slice((*T)(unsafe.Pointer(ptr)), sz)
}

func toCdataStream(ptr *C.struct_ArrowArrayStream) *cdata.CArrowArrayStream {
	return (*cdata.CArrowArrayStream)(unsafe.Pointer(ptr))
}

func toCdataSchema(ptr *C.struct_ArrowSchema) *cdata.CArrowSchema {
	return (*cdata.CArrowSchema)(unsafe.Pointer(ptr))
}

func toCdataArray(ptr *C.struct_ArrowArray) *cdata.CArrowArray {
	return (*cdata.CArrowArray)(unsafe.Pointer(ptr))
}

//export SnowflakeConnectionGetInfo
func SnowflakeConnectionGetInfo(cnxn *C.struct_AdbcConnection, codes *C.uint32_t, len C.size_t, out *C.struct_ArrowArrayStream, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcConnectionGetInfo")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	infoCodes := fromCArr[adbc.InfoCode](codes, int(len))
	rdr, e := conn.cnxn.GetInfo(context.Background(), infoCodes)
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}

	cdata.ExportRecordReader(rdr, toCdataStream(out))
	return C.ADBC_STATUS_OK
}

func toStrPtr(in *C.cchar_t) *string {
	if in == nil {
		return nil
	}

	out := C.GoString((*C.char)(in))
	return &out
}

func toStrSlice(in **C.cchar_t) []string {
	if in == nil {
		return nil
	}

	sz := unsafe.Sizeof(*in)

	out := make([]string, 0, 1)
	for *in != nil {
		out = append(out, C.GoString(*in))
		in = (**C.cchar_t)(unsafe.Add(unsafe.Pointer(in), sz))
	}
	return out
}

//export SnowflakeConnectionGetObjects
func SnowflakeConnectionGetObjects(cnxn *C.struct_AdbcConnection, depth C.int, catalog, dbSchema, tableName *C.cchar_t, tableType **C.cchar_t, columnName *C.cchar_t,
	out *C.struct_ArrowArrayStream, err *C.struct_AdbcError) C.AdbcStatusCode {

	conn := checkConnInit(cnxn, err, "AdbcConnectionGetObjects")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	rdr, e := conn.cnxn.GetObjects(context.Background(), adbc.ObjectDepth(depth), toStrPtr(catalog), toStrPtr(dbSchema), toStrPtr(tableName), toStrPtr(columnName), toStrSlice(tableType))
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}
	cdata.ExportRecordReader(rdr, toCdataStream(out))
	return C.ADBC_STATUS_OK
}

//export SnowflakeConnectionGetTableSchema
func SnowflakeConnectionGetTableSchema(cnxn *C.struct_AdbcConnection, catalog, dbSchema, tableName *C.cchar_t, schema *C.struct_ArrowSchema, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcConnectionGetTableSchema")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	sc, e := conn.cnxn.GetTableSchema(context.Background(), toStrPtr(catalog), toStrPtr(dbSchema), C.GoString(tableName))
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}
	cdata.ExportArrowSchema(sc, toCdataSchema(schema))
	return C.ADBC_STATUS_OK
}

//export SnowflakeConnectionGetTableTypes
func SnowflakeConnectionGetTableTypes(cnxn *C.struct_AdbcConnection, out *C.struct_ArrowArrayStream, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcConnectionGetTableTypes")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	rdr, e := conn.cnxn.GetTableTypes(context.Background())
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}
	cdata.ExportRecordReader(rdr, toCdataStream(out))
	return C.ADBC_STATUS_OK
}

//export SnowflakeConnectionReadPartition
func SnowflakeConnectionReadPartition(cnxn *C.struct_AdbcConnection, serialized *C.cuint8_t, serializedLen C.size_t, out *C.struct_ArrowArrayStream, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcConnectionReadPartition")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	rdr, e := conn.cnxn.ReadPartition(context.Background(), fromCArr[byte](serialized, int(serializedLen)))
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}
	cdata.ExportRecordReader(rdr, toCdataStream(out))
	return C.ADBC_STATUS_OK
}

//export SnowflakeConnectionCommit
func SnowflakeConnectionCommit(cnxn *C.struct_AdbcConnection, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcConnectionCommit")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	return C.AdbcStatusCode(errToAdbcErr(err, conn.cnxn.Commit(context.Background())))
}

//export SnowflakeConnectionRollback
func SnowflakeConnectionRollback(cnxn *C.struct_AdbcConnection, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcConnectionRollback")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	return C.AdbcStatusCode(errToAdbcErr(err, conn.cnxn.Rollback(context.Background())))
}

func checkStmtInit(stmt *C.struct_AdbcStatement, err *C.struct_AdbcError, fname string) adbc.Statement {
	if stmt == nil {
		setErr(err, "%s: statement not allocated", fname)
		return nil
	}

	if stmt.private_data == nil {
		setErr(err, "%s: statement not initialized", fname)
		return nil
	}

	return (*(*cgo.Handle)(stmt.private_data)).Value().(adbc.Statement)
}

//export SnowflakeStatementNew
func SnowflakeStatementNew(cnxn *C.struct_AdbcConnection, stmt *C.struct_AdbcStatement, err *C.struct_AdbcError) C.AdbcStatusCode {
	conn := checkConnInit(cnxn, err, "AdbcStatementNew")
	if conn == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	st, e := conn.cnxn.NewStatement()
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}

	h := cgo.NewHandle(st)
	stmt.private_data = createHandle(h)
	return C.ADBC_STATUS_OK
}

//export SnowflakeStatementRelease
func SnowflakeStatementRelease(stmt *C.struct_AdbcStatement, err *C.struct_AdbcError) C.AdbcStatusCode {
	if stmt == nil {
		setErr(err, "AdbcStatementRelease: statement not allocated")
		return C.ADBC_STATUS_INVALID_STATE
	}

	if stmt.private_data == nil {
		setErr(err, "AdbcStatementRelease: statement not initialized")
		return C.ADBC_STATUS_INVALID_STATE
	}

	h := (*(*cgo.Handle)(stmt.private_data))
	st := h.Value().(adbc.Statement)
	C.free(stmt.private_data)
	stmt.private_data = nil

	e := st.Close()
	h.Delete()
	// manually trigger GC for two reasons:
	//  1. ASAN expects the release callback to be called before
	//     the process ends, but GC is not deterministic. So by manually
	//     triggering the GC we ensure the release callback gets called.
	//  2. Creates deterministic GC behavior by all Release functions
	//     triggering a garbage collection
	runtime.GC()
	return C.AdbcStatusCode(errToAdbcErr(err, e))
}

//export SnowflakeStatementPrepare
func SnowflakeStatementPrepare(stmt *C.struct_AdbcStatement, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementPrepare")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	return C.AdbcStatusCode(errToAdbcErr(err, st.Prepare(context.Background())))
}

//export SnowflakeStatementExecuteQuery
func SnowflakeStatementExecuteQuery(stmt *C.struct_AdbcStatement, out *C.struct_ArrowArrayStream, affected *C.int64_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementExecuteQuery")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	if out == nil {
		n, e := st.ExecuteUpdate(context.Background())
		if e != nil {
			return C.AdbcStatusCode(errToAdbcErr(err, e))
		}

		if affected != nil {
			*affected = C.int64_t(n)
		}
	} else {
		rdr, n, e := st.ExecuteQuery(context.Background())
		if e != nil {
			return C.AdbcStatusCode(errToAdbcErr(err, e))
		}

		if affected != nil {
			*affected = C.int64_t(n)
		}

		cdata.ExportRecordReader(rdr, toCdataStream(out))
	}
	return C.ADBC_STATUS_OK
}

//export SnowflakeStatementSetSqlQuery
func SnowflakeStatementSetSqlQuery(stmt *C.struct_AdbcStatement, query *C.cchar_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementSetSqlQuery")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	return C.AdbcStatusCode(errToAdbcErr(err, st.SetSqlQuery(C.GoString(query))))
}

//export SnowflakeStatementSetSubstraitPlan
func SnowflakeStatementSetSubstraitPlan(stmt *C.struct_AdbcStatement, plan *C.cuint8_t, length C.size_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementSetSubstraitPlan")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	return C.AdbcStatusCode(errToAdbcErr(err, st.SetSubstraitPlan(fromCArr[byte](plan, int(length)))))
}

//export SnowflakeStatementBind
func SnowflakeStatementBind(stmt *C.struct_AdbcStatement, values *C.struct_ArrowArray, schema *C.struct_ArrowSchema, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementBind")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	rec, e := cdata.ImportCRecordBatch(toCdataArray(values), toCdataSchema(schema))
	if e != nil {
		// if there was an error, we need to manually release the input
		cdata.ReleaseCArrowArray(toCdataArray(values))
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}
	defer rec.Release()

	return C.AdbcStatusCode(errToAdbcErr(err, st.Bind(context.Background(), rec)))
}

//export SnowflakeStatementBindStream
func SnowflakeStatementBindStream(stmt *C.struct_AdbcStatement, stream *C.struct_ArrowArrayStream, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementBindStream")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	rdr := cdata.ImportCArrayStream(toCdataStream(stream), nil)
	return C.AdbcStatusCode(errToAdbcErr(err, st.BindStream(context.Background(), rdr.(array.RecordReader))))
}

//export SnowflakeStatementGetParameterSchema
func SnowflakeStatementGetParameterSchema(stmt *C.struct_AdbcStatement, schema *C.struct_ArrowSchema, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementGetParameterSchema")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	sc, e := st.GetParameterSchema()
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}

	cdata.ExportArrowSchema(sc, toCdataSchema(schema))
	return C.ADBC_STATUS_OK
}

//export SnowflakeStatementSetOption
func SnowflakeStatementSetOption(stmt *C.struct_AdbcStatement, key, value *C.cchar_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementSetOption")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	return C.AdbcStatusCode(errToAdbcErr(err, st.SetOption(C.GoString(key), C.GoString(value))))
}

//export releasePartitions
func releasePartitions(partitions *C.struct_AdbcPartitions) {
	if partitions.private_data == nil {
		return
	}

	C.free(unsafe.Pointer(partitions.partitions))
	C.free(unsafe.Pointer(partitions.partition_lengths))
	C.free(partitions.private_data)
	partitions.partitions = nil
	partitions.partition_lengths = nil
	partitions.private_data = nil
}

//export SnowflakeStatementExecutePartitions
func SnowflakeStatementExecutePartitions(stmt *C.struct_AdbcStatement, schema *C.struct_ArrowSchema, partitions *C.struct_AdbcPartitions, affected *C.int64_t, err *C.struct_AdbcError) C.AdbcStatusCode {
	st := checkStmtInit(stmt, err, "AdbcStatementExecutePartitions")
	if st == nil {
		return C.ADBC_STATUS_INVALID_STATE
	}

	sc, part, n, e := st.ExecutePartitions(context.Background())
	if e != nil {
		return C.AdbcStatusCode(errToAdbcErr(err, e))
	}

	if partitions == nil {
		setErr(err, "AdbcStatementExecutePartitions: partitions output struct is null")
		return C.ADBC_STATUS_INVALID_ARGUMENT
	}

	if affected != nil {
		*affected = C.int64_t(n)
	}

	if sc != nil && schema != nil {
		cdata.ExportArrowSchema(sc, toCdataSchema(schema))
	}

	partitions.num_partitions = C.size_t(part.NumPartitions)
	partitions.partitions = (**C.cuint8_t)(C.malloc(C.size_t(unsafe.Sizeof((*C.uint8_t)(nil)) * uintptr(part.NumPartitions))))
	partitions.partition_lengths = (*C.size_t)(C.malloc(C.size_t(unsafe.Sizeof(C.size_t(0)) * uintptr(part.NumPartitions))))

	// Copy into C-allocated memory to avoid violating CGO rules
	totalLen := 0
	for _, p := range part.PartitionIDs {
		totalLen += len(p)
	}
	partitions.private_data = C.malloc(C.size_t(totalLen))
	dst := fromCArr[byte]((*byte)(partitions.private_data), totalLen)

	partIDs := fromCArr[*C.cuint8_t](partitions.partitions, int(partitions.num_partitions))
	partLens := fromCArr[C.size_t](partitions.partition_lengths, int(partitions.num_partitions))
	for i, p := range part.PartitionIDs {
		partIDs[i] = (*C.cuint8_t)(&dst[0])
		copy(dst, p)
		dst = dst[len(p):]
		partLens[i] = C.size_t(len(p))
	}

	partitions.release = (*[0]byte)(C.releasePartitions)
	return C.ADBC_STATUS_OK
}

//export SnowflakeDriverInit
func SnowflakeDriverInit(version C.int, rawDriver *C.void, err *C.struct_AdbcError) C.AdbcStatusCode {
	if version != C.ADBC_VERSION_1_0_0 {
		setErr(err, "Only version %d supported, got %d", int(C.ADBC_VERSION_1_0_0), int(version))
		return C.ADBC_STATUS_NOT_IMPLEMENTED
	}

	driver := (*C.struct_AdbcDriver)(unsafe.Pointer(rawDriver))
	C.memset(unsafe.Pointer(driver), 0, C.sizeof_struct_AdbcDriver)
	driver.DatabaseInit = (*[0]byte)(C.SnowflakeDatabaseInit)
	driver.DatabaseNew = (*[0]byte)(C.SnowflakeDatabaseNew)
	driver.DatabaseRelease = (*[0]byte)(C.SnowflakeDatabaseRelease)
	driver.DatabaseSetOption = (*[0]byte)(C.SnowflakeDatabaseSetOption)

	driver.ConnectionNew = (*[0]byte)(C.SnowflakeConnectionNew)
	driver.ConnectionInit = (*[0]byte)(C.SnowflakeConnectionInit)
	driver.ConnectionRelease = (*[0]byte)(C.SnowflakeConnectionRelease)
	driver.ConnectionSetOption = (*[0]byte)(C.SnowflakeConnectionSetOption)
	driver.ConnectionGetInfo = (*[0]byte)(C.SnowflakeConnectionGetInfo)
	driver.ConnectionGetObjects = (*[0]byte)(C.SnowflakeConnectionGetObjects)
	driver.ConnectionGetTableSchema = (*[0]byte)(C.SnowflakeConnectionGetTableSchema)
	driver.ConnectionGetTableTypes = (*[0]byte)(C.SnowflakeConnectionGetTableTypes)
	driver.ConnectionReadPartition = (*[0]byte)(C.SnowflakeConnectionReadPartition)
	driver.ConnectionCommit = (*[0]byte)(C.SnowflakeConnectionCommit)
	driver.ConnectionRollback = (*[0]byte)(C.SnowflakeConnectionRollback)

	driver.StatementNew = (*[0]byte)(C.SnowflakeStatementNew)
	driver.StatementRelease = (*[0]byte)(C.SnowflakeStatementRelease)
	driver.StatementSetOption = (*[0]byte)(C.SnowflakeStatementSetOption)
	driver.StatementSetSqlQuery = (*[0]byte)(C.SnowflakeStatementSetSqlQuery)
	driver.StatementSetSubstraitPlan = (*[0]byte)(C.SnowflakeStatementSetSubstraitPlan)
	driver.StatementBind = (*[0]byte)(C.SnowflakeStatementBind)
	driver.StatementBindStream = (*[0]byte)(C.SnowflakeStatementBindStream)
	driver.StatementExecuteQuery = (*[0]byte)(C.SnowflakeStatementExecuteQuery)
	driver.StatementExecutePartitions = (*[0]byte)(C.SnowflakeStatementExecutePartitions)
	driver.StatementGetParameterSchema = (*[0]byte)(C.SnowflakeStatementGetParameterSchema)
	driver.StatementPrepare = (*[0]byte)(C.SnowflakeStatementPrepare)

	return C.ADBC_STATUS_OK
}

func main() {}
