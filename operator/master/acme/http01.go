package acme

import (
	"context"
	"fmt"
	"time"

	ingressbuilder "aaa/ingress/builder"
)

type HTTP01Solver struct {
	controller Controller
}

func (s *HTTP01Solver) Present(ctx context.Context, hostname, token, fixContent string) error {
	if err := ensureController(s.controller); err != nil {
		return err
	}
	if hostname == "" || token == "" || fixContent == "" {
		return fmt.Errorf("hostname, token and fixContent are required")
	}
	logACMEStart("http-01 present [%s] token=%s", hostname, token)
	return run(ctx, func(ctx context.Context) error {
		route, err := readHTTPRouteOrEmpty(ctx, s.controller, hostname)
		if err != nil {
			logACMEError(err, "http-01 present [%s] token=%s", hostname, token)
			return err
		}
		challengePolicy := ingressbuilder.NewHTTPPolicy().
			WithExactPath(challengePath(token)).
			WithFixContent(fixContent).
			WithAllowRawAccess(true)
		next := make([]*ingressbuilder.HTTPPolicyBuilder, 0, len(route.Policies)+1)
		next = append(next, challengePolicy)
		for _, policy := range route.Policies {
			if isChallengePolicy(policy, token) {
				continue
			}
			next = append(next, policy)
		}
		route.HostName = hostname
		route.Policies = next
		if err := s.controller.PublishHTTPRoute(ctx, hostname, route); err != nil {
			logACMEError(err, "http-01 present [%s] token=%s", hostname, token)
			return err
		}
		state := challengeState{
			Type:       challengeTypeHTTP01,
			Hostname:   hostname,
			Token:      token,
			FixContent: fixContent,
			CreatedAt:  unixNow(),
		}
		if err := saveChallengeState(s.root(), state, http01Key(hostname, token)); err != nil {
			logACMEError(err, "http-01 present [%s] token=%s", hostname, token)
			return err
		}
		logACMEDone("http-01 present [%s] token=%s", hostname, token)
		return nil
	})
}

func (s *HTTP01Solver) Cleanup(ctx context.Context, hostname, token string) error {
	if err := ensureController(s.controller); err != nil {
		return err
	}
	if hostname == "" || token == "" {
		return fmt.Errorf("hostname and token are required")
	}
	logACMEStart("http-01 cleanup [%s] token=%s", hostname, token)
	return run(ctx, func(ctx context.Context) error {
		state, err := loadChallengeState(s.root(), http01Key(hostname, token))
		if err != nil {
			if isNotExist(err) {
				logACMEDone("http-01 cleanup [%s] token=%s skipped=not-found", hostname, token)
				return nil
			}
			logACMEError(err, "http-01 cleanup [%s] token=%s", hostname, token)
			return err
		}
		route, err := readHTTPRouteOrEmpty(ctx, s.controller, state.Hostname)
		if err != nil {
			logACMEError(err, "http-01 cleanup [%s] token=%s", hostname, token)
			return err
		}
		filtered := make([]*ingressbuilder.HTTPPolicyBuilder, 0, len(route.Policies))
		for _, policy := range route.Policies {
			if isChallengePolicy(policy, state.Token) {
				continue
			}
			filtered = append(filtered, policy)
		}
		route.HostName = state.Hostname
		route.Policies = filtered
		if err := s.controller.PublishHTTPRoute(ctx, state.Hostname, route); err != nil {
			logACMEError(err, "http-01 cleanup [%s] token=%s", hostname, token)
			return err
		}
		if err := deleteChallengeState(s.root(), http01Key(hostname, token)); err != nil {
			logACMEError(err, "http-01 cleanup [%s] token=%s", hostname, token)
			return err
		}
		logACMEDone("http-01 cleanup [%s] token=%s", hostname, token)
		return nil
	})
}

func (s *HTTP01Solver) root() string {
	return s.controller.Root()
}

func http01Key(hostname, token string) string {
	return hostname + "|" + token
}

func challengePath(token string) string {
	return "/.well-known/acme-challenge/" + token
}

func isChallengePolicy(policy *ingressbuilder.HTTPPolicyBuilder, token string) bool {
	return policy != nil &&
		policy.PathnameKind.String() == "exact" &&
		policy.Pathname == challengePath(token) &&
		policy.FixContent != ""
}

func readHTTPRouteOrEmpty(ctx context.Context, c Controller, hostname string) (*ingressbuilder.HTTPRouteBuilder, error) {
	route, err := c.ReadHTTPRoute(ctx, hostname)
	if err != nil {
		if isNotExist(err) {
			return ingressbuilder.NewHTTPRoute().WithHostName(hostname), nil
		}
		return nil, err
	}
	return route, nil
}

func unixNow() int64 {
	return time.Now().Unix()
}
