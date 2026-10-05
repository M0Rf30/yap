// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

#include <stdio.h>

// Forward declaration - package-b depends on package-a
extern void package_a_function(void);

void package_b_function(void) {
    printf("Package B function called\n");
    // This creates the circular dependency
    package_a_function();
}

int main() {
    package_b_function();
    return 0;
}