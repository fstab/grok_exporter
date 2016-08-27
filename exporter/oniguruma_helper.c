#include "oniguruma_helper.h"

// CGO does not support C preprocessor instructions (#if, #else, #endif).
// This initialization wrapper is to be compatible with both, Onituruma 5.9.6 and Oniguruma 6.0.0.

int oniguruma_helper_initialize(OnigEncoding encodings[], int n) {
    #if ONIGURUMA_VERSION_MAJOR == 6
        return onig_initialize(encodings, n);
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
