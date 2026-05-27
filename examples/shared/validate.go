package shared

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateNameNamespace checks that name begins with a recognised namespace
// prefix (d/, id/, or p/) and returns an error if it does not.
func ValidateNameNamespace(name string) error {
	if !strings.HasPrefix(name, "d/") && !strings.HasPrefix(name, "id/") && !strings.HasPrefix(name, "p/") {
		return fmt.Errorf("name must start with d/, id/, or p/ namespace (got %q)", name)
	}
	return nil
}

// ValidateNameValue validates that value is well-formed for the given name's
// namespace: d/ and id/ names require a valid JSON value.
func ValidateNameValue(name, value string) error {
	if strings.HasPrefix(name, "d/") || strings.HasPrefix(name, "id/") {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(value), &js); err != nil {
			ns := strings.Split(name, "/")[0] + "/"
			return fmt.Errorf("value must be valid JSON for %s namespace: %w", ns, err)
		}
	}
	return nil
}
