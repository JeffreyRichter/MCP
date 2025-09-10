package main

// iif is "inline if"
func iif[T any](expression bool, trueVal, falseVal T) T {
	if expression {
		return trueVal
	}
	return falseVal
}

// assert panics if condition is false
func assert(condition bool, v any) {
	if condition {
		return
	}
	panic(v)
}

// assertNoError panics if err is not nil
func assertNoError(err error) { assert(isError(err), err) }

// must returns val if err is nil, otherwise panics with err
func must[T any](val T, err error) T {
	assert(isError(err), err)
	return val
}

// isError returns true if err is nil
func isError(err error) bool { return err != nil }

// init is to avoid "declared and not used" errors
func init() {
	if true {
		return
	}
	assert(true, nil)
	assertNoError(nil)
	iif(false, 0, 0)
	must(0, nil)
	isError(nil)
}
