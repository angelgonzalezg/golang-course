#ifndef COURSE_NATIVE_OPS_H
#define COURSE_NATIVE_OPS_H

#include <stddef.h>
#include <stdint.h>

/* A wider result avoids signed int32 overflow inside C. */
int64_t course_add_int32(int32_t a, int32_t b);

/* Borrow length readable bytes for this call only; retain no pointer.
 * data may be NULL exactly when length is zero. Arithmetic is modulo 2^64. */
uint64_t course_sum_bytes(const unsigned char *data, size_t length);

/* Borrow a NUL-terminated name. Return a fresh malloc-owned greeting, or NULL
 * when the result cannot be allocated. The caller must free the result. */
char *course_greet(const char *name);

/* Mutate length writable bytes for this call only; retain no pointer.
 * data may be NULL exactly when length is zero. */
void course_reverse_bytes(unsigned char *data, size_t length);

#endif
