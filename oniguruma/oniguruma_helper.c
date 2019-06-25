// Copyright 2016-2018 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "oniguruma_helper.h"

// CGO does not support C preprocessor instructions (#if, #else, #endif).
// This initialization wrapper is to be compatible with both, Onituruma 5.9.6 and Oniguruma 6.0.0.

int oniguruma_helper_initialize(OnigEncoding encodings[], int n) {
    #if ONIGURUMA_VERSION_MAJOR >= 6
        int result = onig_initialize(encodings, n);
        #if ONIGURUMA_VERSION_MINOR >= 8 && ONIGURUMA_VERSION_TEENY >= 2
            // Increase the retry limit by factor 100 to make the examples in #58 work.
            // However, this value is ridiculously high, regular expressions needing this will be unreasonably slow.
            // See here for documentation: https://github.com/kkos/oniguruma/issues/143
            onig_set_retry_limit_in_match(100L*onig_get_retry_limit_in_match());
        #endif
        return result;
    #else
        return 0;
    #endif
}

// GGO cannot call call C functions with varargs.
// As a workaround, we implement helper functions with fixed arguments delegating to Oniguruma's vararg functions.

int oniguruma_helper_error_code_with_info_to_str(UChar* err_buf, int err_code, OnigErrorInfo *errInfo) {
    return onig_error_code_to_str(err_buf, err_code, errInfo);
}

int oniguruma_helper_error_code_to_str(UChar* err_buf, int err_code) {
    return onig_error_code_to_str(err_buf, err_code);
}

int oniguruma_helper_is_retry_limit_error(int err_code) {
    #ifdef ONIGERR_RETRY_LIMIT_IN_MATCH_OVER
        return err_code == ONIGERR_RETRY_LIMIT_IN_MATCH_OVER;
    #else
        return 0;
    #endif
}