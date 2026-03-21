package builder

import (
	"aaa/ingress"
	"fmt"
)

type HTTPRouteBuilder struct {
	Name     string
	Policies []*HTTPPolicyBuilder
}

type HTTPPolicyBuilder struct {
	Backend      string
	PathnameKind ingress.Pathname_Kind
	Pathname     string
	QueryItems   []HTTPKV
	HeaderItems  []HTTPKV
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

func (b *HTTPRouteBuilder) WithName(name string) *HTTPRouteBuilder {
	b.Name = name
	return b
}

func (b *HTTPRouteBuilder) AddPolicy(policy *HTTPPolicyBuilder) *HTTPRouteBuilder {
	b.Policies = append(b.Policies, policy)
	return b
}

func (b *HTTPRouteBuilder) Build(zone ingress.HttpZone) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	if err := zone.SetName(b.Name); err != nil {
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
	if err := requireText("backend", b.Backend); err != nil {
		return err
	}
	if err := policy.SetBackend(b.Backend); err != nil {
		return err
	}

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
