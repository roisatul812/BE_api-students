package helper

import "fmt"

func ParseInt(value string, defaultValue int) int {
	var result int

	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}

	return result
}
