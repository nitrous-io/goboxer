package host

// #include <unistd.h>
import "C"

func FreeMemory() uint64 {
	return uint64(C.sysconf(C._SC_PAGESIZE) * C.sysconf(C._SC_AVPHYS_PAGES))
}
