package acme

import (
	"context"
	"fmt"
	"time"
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
		change, err := readHTTPChangeOrEmpty(ctx, s.controller, hostname)
		if err != nil {
			logACMEError(err, "http-01 present [%s] token=%s", hostname, token)
			return err
		}
		challengePolicy := HTTPPolicy{
			PathnameKind:   "exact",
			Pathname:       challengePath(token),
			FixContent:     fixContent,
			AllowRawAccess: true,
		}
		next := make([]HTTPPolicy, 0, len(change.Policies)+1)
		next = append(next, challengePolicy)
		for _, policy := range change.Policies {
			if isChallengePolicy(policy, token) {
				continue
			}
			next = append(next, policy)
		}
		change.Name = hostname
		change.Policies = next
		if err := s.controller.PublishHTTPChange(ctx, hostname, change); err != nil {
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
		change, err := readHTTPChangeOrEmpty(ctx, s.controller, state.Hostname)
		if err != nil {
			logACMEError(err, "http-01 cleanup [%s] token=%s", hostname, token)
			return err
		}
		filtered := make([]HTTPPolicy, 0, len(change.Policies))
		for _, policy := range change.Policies {
			if isChallengePolicy(policy, state.Token) {
				continue
			}
			filtered = append(filtered, policy)
		}
		change.Name = state.Hostname
		change.Policies = filtered
		if err := s.controller.PublishHTTPChange(ctx, state.Hostname, change); err != nil {
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
	if s.controller == nil || s.controller.Root() == "" {
		return "."
	}
	return s.controller.Root()
}

func http01Key(hostname, token string) string {
	return hostname + "|" + token
}

func challengePath(token string) string {
	return "/.well-known/acme-challenge/" + token
}

func isChallengePolicy(policy HTTPPolicy, token string) bool {
	return policy.PathnameKind == "exact" &&
		policy.Pathname == challengePath(token) &&
		policy.FixContent != ""
}

func readHTTPChangeOrEmpty(ctx context.Context, c Controller, hostname string) (HTTPChange, error) {
	change, err := c.ReadHTTP(ctx, hostname)
	if err != nil {
		if isNotExist(err) {
			return HTTPChange{Name: hostname}, nil
		}
		return HTTPChange{}, err
	}
	return change, nil
}

func unixNow() int64 {
	return time.Now().Unix()
}
