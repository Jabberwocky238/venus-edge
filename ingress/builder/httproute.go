package builder

import (
	"aaa/ingress"
	"fmt"
)

type HTTPRouteBuilder struct {
	HostName string
	Policies []*HTTPPolicyBuilder
}

type HTTPPolicyBuilder struct {
	Backend        string
	PathnameKind   ingress.Pathname_Kind
	Pathname       string
	QueryItems     []HTTPKV
	HeaderItems    []HTTPKV
	FixContent     string
	AllowRawAccess bool
}

type HTTPKV struct {
	Key   string
	Value string
}

func NewHTTPRoute() *HTTPRouteBuilder {
	return &HTTPRouteBuilder{}
}

func NewHTTPPolicy() *HTTPPolicyBuilder {
	return &HTTPPolicyBuilder{}
}

func (b *HTTPRouteBuilder) WithHostName(hostname string) *HTTPRouteBuilder {
	b.HostName = hostname
	return b
}

func (b *HTTPRouteBuilder) Use(other *HTTPRouteBuilder) *HTTPRouteBuilder {
	if other == nil {
		return b
	}
	b.HostName = other.HostName
	if len(other.Policies) == 0 {
		b.Policies = nil
		return b
	}
	b.Policies = make([]*HTTPPolicyBuilder, 0, len(other.Policies))
	for _, policy := range other.Policies {
		if policy == nil {
			b.Policies = append(b.Policies, nil)
			continue
		}
		b.Policies = append(b.Policies, NewHTTPPolicy().Use(policy))
	}
	return b
}

func (b *HTTPRouteBuilder) From(zone ingress.HttpZone) error {
	name, err := zone.Hostname()
	if err != nil {
		return err
	}
	list, err := zone.HttpPolicies()
	if err != nil {
		return err
	}

	b.HostName = name
	if list.Len() == 0 {
		b.Policies = nil
		return nil
	}
	b.Policies = make([]*HTTPPolicyBuilder, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		policy := NewHTTPPolicy()
		if err := policy.From(list.At(i)); err != nil {
			return fmt.Errorf("decode policy %d: %w", i, err)
		}
		b.Policies = append(b.Policies, policy)
	}
	return nil
}

func (b *HTTPRouteBuilder) AddPolicy(policy *HTTPPolicyBuilder) *HTTPRouteBuilder {
	b.Policies = append(b.Policies, policy)
	return b
}

func (b *HTTPRouteBuilder) Build(zone ingress.HttpZone) error {
	if err := requireText("hostname", b.HostName); err != nil {
		return err
	}
	if err := zone.SetHostname(b.HostName); err != nil {
		return err
	}
	list, err := zone.NewHttpPolicies(int32(len(b.Policies)))
	if err != nil {
		return err
	}
	for i, policy := range b.Policies {
		if policy == nil {
			return fmt.Errorf("policy[%d] is required", i)
		}
		if err := policy.Build(list.At(i)); err != nil {
			return fmt.Errorf("build policy %d: %w", i, err)
		}
	}
	return nil
}

func (b *HTTPPolicyBuilder) WithBackend(backend string) *HTTPPolicyBuilder {
	b.Backend = backend
	return b
}

func (b *HTTPPolicyBuilder) Use(other *HTTPPolicyBuilder) *HTTPPolicyBuilder {
	if other == nil {
		return b
	}
	b.Backend = other.Backend
	b.PathnameKind = other.PathnameKind
	b.Pathname = other.Pathname
	b.FixContent = other.FixContent
	b.AllowRawAccess = other.AllowRawAccess
	if len(other.QueryItems) == 0 {
		b.QueryItems = nil
	} else {
		b.QueryItems = append([]HTTPKV(nil), other.QueryItems...)
	}
	if len(other.HeaderItems) == 0 {
		b.HeaderItems = nil
	} else {
		b.HeaderItems = append([]HTTPKV(nil), other.HeaderItems...)
	}
	return b
}

func (b *HTTPPolicyBuilder) From(policy ingress.HttpPolicy) error {
	backend, err := policy.Backend()
	if err != nil {
		return err
	}
	fixContent, err := policy.FixContent()
	if err != nil {
		return err
	}

	b.Backend = backend
	b.FixContent = fixContent
	b.AllowRawAccess = policy.AllowRawAccess()
	b.Pathname = ""
	b.QueryItems = nil
	b.HeaderItems = nil

	switch policy.Which() {
	case ingress.HttpPolicy_Which_pathname:
		pathname, err := policy.Pathname()
		if err != nil {
			return err
		}
		b.PathnameKind = pathname.Kind()
		switch pathname.Which() {
		case ingress.Pathname_Which_exact:
			b.Pathname, err = pathname.Exact()
		case ingress.Pathname_Which_prefix:
			b.Pathname, err = pathname.Prefix()
		case ingress.Pathname_Which_regex:
			b.Pathname, err = pathname.Regex()
		default:
			return fmt.Errorf("unsupported pathname union: %v", pathname.Which())
		}
		if err != nil {
			return err
		}
	case ingress.HttpPolicy_Which_query:
		query, err := policy.Query()
		if err != nil {
			return err
		}
		items, err := query.Items()
		if err != nil {
			return err
		}
		b.QueryItems = make([]HTTPKV, 0, items.Len())
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			key, err := item.Key()
			if err != nil {
				return err
			}
			value, err := item.Value()
			if err != nil {
				return err
			}
			b.QueryItems = append(b.QueryItems, HTTPKV{Key: key, Value: value})
		}
	case ingress.HttpPolicy_Which_header:
		header, err := policy.Header()
		if err != nil {
			return err
		}
		items, err := header.Items()
		if err != nil {
			return err
		}
		b.HeaderItems = make([]HTTPKV, 0, items.Len())
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			key, err := item.Key()
			if err != nil {
				return err
			}
			value, err := item.Value()
			if err != nil {
				return err
			}
			b.HeaderItems = append(b.HeaderItems, HTTPKV{Key: key, Value: value})
		}
	default:
		return fmt.Errorf("unsupported http policy union: %v", policy.Which())
	}

	return nil
}

func (b *HTTPPolicyBuilder) WithFixContent(fixContent string) *HTTPPolicyBuilder {
	b.FixContent = fixContent
	return b
}

func (b *HTTPPolicyBuilder) WithAllowRawAccess(allow bool) *HTTPPolicyBuilder {
	b.AllowRawAccess = allow
	return b
}

func (b *HTTPPolicyBuilder) WithExactPath(path string) *HTTPPolicyBuilder {
	b.PathnameKind = ingress.Pathname_Kind_exact
	b.Pathname = path
	b.QueryItems = nil
	b.HeaderItems = nil
	return b
}

func (b *HTTPPolicyBuilder) WithPrefixPath(path string) *HTTPPolicyBuilder {
	b.PathnameKind = ingress.Pathname_Kind_prefix
	b.Pathname = path
	b.QueryItems = nil
	b.HeaderItems = nil
	return b
}

func (b *HTTPPolicyBuilder) WithRegexPath(expr string) *HTTPPolicyBuilder {
	b.PathnameKind = ingress.Pathname_Kind_regex
	b.Pathname = expr
	b.QueryItems = nil
	b.HeaderItems = nil
	return b
}

func (b *HTTPPolicyBuilder) WithQuery(key, value string) *HTTPPolicyBuilder {
	b.Pathname = ""
	b.QueryItems = append(b.QueryItems, HTTPKV{Key: key, Value: value})
	b.HeaderItems = nil
	return b
}

func (b *HTTPPolicyBuilder) WithHeader(key, value string) *HTTPPolicyBuilder {
	b.Pathname = ""
	b.QueryItems = nil
	b.HeaderItems = append(b.HeaderItems, HTTPKV{Key: key, Value: value})
	return b
}

func (b *HTTPPolicyBuilder) Build(policy ingress.HttpPolicy) error {
	if err := policy.SetBackend(b.Backend); err != nil {
		return err
	}
	if b.Backend == "" && b.FixContent == "" {
		return fmt.Errorf("one of backend or fixContent is required")
	}
	if b.FixContent != "" {
		if err := policy.SetFixContent(b.FixContent); err != nil {
			return err
		}
	}
	policy.SetAllowRawAccess(b.AllowRawAccess)

	switch {
	case b.Pathname != "":
		pathname, err := policy.NewPathname()
		if err != nil {
			return err
		}
		pathname.SetKind(b.PathnameKind)
		switch b.PathnameKind {
		case ingress.Pathname_Kind_exact:
			return pathname.SetExact(b.Pathname)
		case ingress.Pathname_Kind_prefix:
			return pathname.SetPrefix(b.Pathname)
		case ingress.Pathname_Kind_regex:
			return pathname.SetRegex(b.Pathname)
		default:
			return fmt.Errorf("unsupported pathname kind: %v", b.PathnameKind)
		}
	case len(b.QueryItems) > 0:
		query, err := policy.NewQuery()
		if err != nil {
			return err
		}
		return setKeyValues(query.NewItems, b.QueryItems)
	case len(b.HeaderItems) > 0:
		header, err := policy.NewHeader()
		if err != nil {
			return err
		}
		return setKeyValues(header.NewItems, b.HeaderItems)
	default:
		return fmt.Errorf("one of pathname, query, header is required")
	}
}

type keyValueListBuilder func(int32) (ingress.KeyValue_List, error)

func setKeyValues(newList keyValueListBuilder, items []HTTPKV) error {
	list, err := newList(int32(len(items)))
	if err != nil {
		return err
	}
	for i, item := range items {
		if err := requireText(fmt.Sprintf("items[%d].key", i), item.Key); err != nil {
			return err
		}
		if err := requireText(fmt.Sprintf("items[%d].value", i), item.Value); err != nil {
			return err
		}
		entry := list.At(i)
		if err := entry.SetKey(item.Key); err != nil {
			return err
		}
		if err := entry.SetValue(item.Value); err != nil {
			return err
		}
	}
	return nil
}
