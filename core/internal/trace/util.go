package trace

import "runtime"

func getCaller() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return ""
	}
	fn := runtime.FuncForPC(pc)
	return fn.Name()
}
