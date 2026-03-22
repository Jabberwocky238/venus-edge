package dns

import (
	"fmt"
	"net"
	"os"

	mdns "github.com/miekg/dns"
)

func logDNSRequest(addr net.Addr, req *mdns.Msg) {
	question := "-"
	if req != nil && len(req.Question) > 0 {
		q := req.Question[0]
		question = fmt.Sprintf("%s %s", q.Name, mdns.TypeToString[q.Qtype])
	}
	fmt.Fprintf(os.Stderr, "%s query from=%s question=%s\n", dnsLogPrefix, formatRemoteAddr(addr), question)
}

func logDNSResponse(addr net.Addr, req, resp *mdns.Msg, err error) {
	question := "-"
	if req != nil && len(req.Question) > 0 {
		q := req.Question[0]
		question = fmt.Sprintf("%s %s", q.Name, mdns.TypeToString[q.Qtype])
	}
	if err != nil || (resp != nil && resp.Rcode != mdns.RcodeSuccess) {
		rcode := mdns.RcodeSuccess
		if resp != nil {
			rcode = resp.Rcode
		}
		fmt.Fprintf(
			os.Stderr,
			"%s %sfail%s to=%s question=%s rcode=%s err=%v\n",
			dnsLogPrefix,
			logColorFail,
			logColorReset,
			formatRemoteAddr(addr),
			question,
			mdns.RcodeToString[rcode],
			err,
		)
		return
	}
	answerCount := 0
	if resp != nil {
		answerCount = len(resp.Answer)
	}
	fmt.Fprintf(
		os.Stderr,
		"%s %ssuccess%s to=%s question=%s answers=%d\n",
		dnsLogPrefix,
		logColorOK,
		logColorReset,
		formatRemoteAddr(addr),
		question,
		answerCount,
	)
}

func formatRemoteAddr(addr net.Addr) string {
	if addr == nil {
		return "-"
	}
	return addr.String()
}
