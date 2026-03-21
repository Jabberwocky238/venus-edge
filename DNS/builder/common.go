package builder

import (
	"fmt"
	"strings"

	dns "aaa/DNS"
)

const DefaultTTL uint32 = 604800

type Record interface {
	Type() dns.RecordType
	Build(record dns.DnsRecord) error
}

func requireText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func requireTexts(field string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s is required", field)
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s[%d] is required", field, i)
		}
	}
	return nil
}
