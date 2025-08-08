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