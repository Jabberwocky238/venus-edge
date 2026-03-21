package ingress

import "aaa/ingress/schema"

type TlsZone = schema.TlsZone
type TlsPolicy = schema.TlsPolicy
type TlsPolicy_Kind = schema.TlsPolicy_Kind
type BackendRef = schema.BackendRef

type HttpZone = schema.HttpZone
type HttpPolicy = schema.HttpPolicy
type HttpPolicy_Which = schema.HttpPolicy_Which
type Pathname = schema.Pathname
type Pathname_Which = schema.Pathname_Which
type Pathname_Kind = schema.Pathname_Kind
type Query = schema.Query
type Header = schema.Header
type KeyValue = schema.KeyValue
type KeyValue_List = schema.KeyValue_List

const (
	TlsPolicy_Kind_tlsPassthrough = schema.TlsPolicy_Kind_tlsPassthrough
	TlsPolicy_Kind_tlsTerminate   = schema.TlsPolicy_Kind_tlsTerminate
	TlsPolicy_Kind_https          = schema.TlsPolicy_Kind_https

	HttpPolicy_Which_pathname = schema.HttpPolicy_Which_pathname
	HttpPolicy_Which_query    = schema.HttpPolicy_Which_query
	HttpPolicy_Which_header   = schema.HttpPolicy_Which_header

	Pathname_Which_exact  = schema.Pathname_Which_exact
	Pathname_Which_prefix = schema.Pathname_Which_prefix
	Pathname_Which_regex  = schema.Pathname_Which_regex

	Pathname_Kind_exact  = schema.Pathname_Kind_exact
	Pathname_Kind_prefix = schema.Pathname_Kind_prefix
	Pathname_Kind_regex  = schema.Pathname_Kind_regex
)

var (
	NewRootTlsZone   = schema.NewRootTlsZone
	ReadRootTlsZone  = schema.ReadRootTlsZone
	NewRootHttpZone  = schema.NewRootHttpZone
	ReadRootHttpZone = schema.ReadRootHttpZone
)
