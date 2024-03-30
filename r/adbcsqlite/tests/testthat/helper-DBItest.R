# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements.  See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership.  The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License.  You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

if (identical(Sys.getenv("NOT_CRAN"), "true") &&
    packageVersion("DBItest") >= "1.7.2") {
  # Needed here to support DBItest::test_some()
  DBItest::make_context(
    adbi::adbi("adbcsqlite"),
    list(
      uri = tempfile("DBItest", fileext = ".sqlite"),
      rows_affected_callback = function() function(x) {
        if (x == -1) testthat::skip("unknown number of `rows_affected`") else x
      }
    ),
    tweaks = suppressWarnings(
      DBItest::tweaks(
        dbitest_version = "1.8.0",
        constructor_relax_args = TRUE,
        placeholder_pattern = c("?", "$1", "$name", ":name"),
        date_cast = function(x) paste0("'", x, "'"),
        time_cast = function(x) paste0("'", x, "'"),
        timestamp_cast = function(x) paste0("'", x, "'"),
        logical_return = function(x) as.integer(x),
        date_typed = FALSE,
        time_typed = FALSE,
        timestamp_typed = FALSE,
        temporary_tables = FALSE, # apache/arrow-adbc#1141
        strict_identifier = TRUE
      )
    ),
    name = "adbcsqlite"
  )
}
