package rabbitmqclient

import "github.com/google/go-cmp/cmp"

// IsBoolEqualToBoolPtr compares a *bool with bool
func IsBoolEqualToBoolPtr(bp *bool, b bool) bool {
	if bp != nil {
		if !cmp.Equal(*bp, b) {
			return false
		}
	}
	return true
}

func IsBoolPtrEqualToBool(bp *bool, b bool) bool {
	if bp != nil {
		if !cmp.Equal(*bp, b) {
			return false
		}
	}
	return true
}

// IsIntEqualToIntPtr compares an *int with int
func IsIntEqualToIntPtr(ip *int, i int) bool {
	if ip != nil {
		if !cmp.Equal(*ip, i) {
			return false
		}
	}
	return true
}

// IsStringEqualToStringPtr compares a string with *string
func IsStringEqualToStringPtr(sp *string, s string) bool {
	if sp != nil {
		if !cmp.Equal(*sp, s) {
			return false
		}
	}
	return true
}

// IsStringPtrEqualToString compares a *string with string
func IsStringPtrEqualToString(sp *string, s string) bool {
	if sp != nil {
		if !cmp.Equal(*sp, s) {
			return false
		}
	}
	return true
}
