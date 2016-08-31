#include <oniguruma.h>

extern int oniguruma_helper_initialize(OnigEncoding encodings[], int n);
extern int oniguruma_helper_error_code_with_info_to_str(UChar* err_buf, int err_code, OnigErrorInfo *errInfo);
extern int oniguruma_helper_error_code_to_str(UChar* err_buf, int err_code);
