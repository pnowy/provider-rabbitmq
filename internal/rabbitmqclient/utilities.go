package rabbitmqclient

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
)

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

// ConvertStringMaptoInterfaceMap convert map[string]string to map[string]interface{}
func ConvertStringMaptoInterfaceMap(m map[string]string) map[string]interface{} {
	finalMap := make(map[string]interface{}, len(m))
	for k, v := range m {
		finalMap[k] = v
	}
	return finalMap
}

// ConvertInterfaceMaptoStringMap convert map[string]interface{} to map[string]string
func ConvertInterfaceMaptoStringMap(m map[string]interface{}) map[string]string {
	finalMap := make(map[string]string)
	for k, v := range m {
		if strValue, ok := v.(string); ok {
			finalMap[k] = strValue
		} else {
			finalMap[k] = fmt.Sprintf("%v", v)
		}
	}
	return finalMap
}

// MapsEqualJSON compares two maps
func MapsEqualJSON(m1, m2 map[string]string) (bool, error) {
	json1, err := json.Marshal(m1)
	if err != nil {
		return false, err
	}
	json2, err := json.Marshal(m2)
	if err != nil {
		return false, err
	}
	return bytes.Equal(json1, json2), err
}
