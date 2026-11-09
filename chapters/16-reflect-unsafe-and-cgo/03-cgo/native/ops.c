#include "ops.h"

#include <stdlib.h>
#include <string.h>

int64_t course_add_int32(int32_t a, int32_t b) {
    return (int64_t)a + (int64_t)b;
}

uint64_t course_sum_bytes(const unsigned char *data, size_t length) {
    uint64_t sum = 0;
    for (size_t index = 0; index < length; index++) {
        sum += data[index];
    }
    return sum;
}

char *course_greet(const char *name) {
    const char prefix[] = "Hello, ";
    const size_t prefix_length = sizeof(prefix) - 1;
    if (name == NULL) {
        return NULL;
    }
    const size_t name_length = strlen(name);
    if (name_length > SIZE_MAX - prefix_length - 1) {
        return NULL;
    }
    char *result = malloc(prefix_length + name_length + 1);
    if (result == NULL) {
        return NULL;
    }
    memcpy(result, prefix, prefix_length);
    memcpy(result + prefix_length, name, name_length + 1); /* Include the NUL. */
    return result;
}

void course_reverse_bytes(unsigned char *data, size_t length) {
    for (size_t index = 0; index < length / 2; index++) {
        const unsigned char temporary = data[index];
        data[index] = data[length - 1 - index];
        data[length - 1 - index] = temporary;
    }
}
