#include <stdio.h>

// Forward declaration - package-a depends on package-b
extern void package_b_function(void);

void package_a_function(void) {
    printf("Package A function called\n");
    // This creates the circular dependency
    package_b_function();
}

int main() {
    package_a_function();
    return 0;
}