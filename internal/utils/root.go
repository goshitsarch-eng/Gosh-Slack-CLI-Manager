package utils

import "os"

// IsRoot reports whether the process runs with an effective UID of 0.
func IsRoot() bool { return os.Geteuid() == 0 }
