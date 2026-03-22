package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	master "aaa/operator/master"
)

type HTTP01Solver struct {
	master *master.Master
}

func (s *HTTP01Solver) Present(ctx context.Context, hostname, token, fixContent string) error {
	if err := ensureMaster(s.master); err != nil {
		return err
	}
	if hostname == "" || token == "" || fixContent == "" {
		return fmt.Errorf("hostname, token and fixContent are required")
	}
	return run(ctx, func(ctx context.Context) error {
		change, err := readHTTPChangeOrEmpty(ctx, s.master, hostname)
		if err != nil {
			return err
		}
		challengePolicy := master.HTTPPolicyJSON{
			PathnameKind:   "exact",
			Pathname:       challengePath(token),
			FixContent:     fixContent,
			AllowRawAccess: true,
		}
		next := make([]master.HTTPPolicyJSON, 0, len(change.Policies)+1)
		next = append(next, challengePolicy)
		for _, policy := range change.Policies {
			if isChallengePolicy(policy, token) {
				continue
			}
			next = append(next, policy)
		}
		change.Name = hostname
		change.Policies = next
		payload, err := json.Marshal(change)
		if err != nil {
			return err
		}
		if _, err := s.master.PublishHTTPJSON(ctx, hostname, payload); err != nil {
			return err
		}
		state := challengeState{
			Type:       challengeTypeHTTP01,
			Hostname:   hostname,
			Token:      token,
			FixContent: fixContent,
			CreatedAt:  unixNow(),
		}
		return saveChallengeState(s.masterRoot(), state, http01Key(hostname, token))
	})
}

func (s *HTTP01Solver) Cleanup(ctx context.Context, hostname, token string) error {
	if err := ensureMaster(s.master); err != nil {
		return err
	}
	if hostname == "" || token == "" {
		return fmt.Errorf("hostname and token are required")
	}
	return run(ctx, func(ctx context.Context) error {
		state, err := loadChallengeState(s.masterRoot(), http01Key(hostname, token))
		if err != nil {
			if isNotExist(err) {
				return nil
			}
			return err
		}
		change, err := readHTTPChangeOrEmpty(ctx, s.master, state.Hostname)
		if err != nil {
			return err
		}
		filtered := make([]master.HTTPPolicyJSON, 0, len(change.Policies))
		for _, policy := range change.Policies {
			if isChallengePolicy(policy, state.Token) {
				continue
			}
			filtered = append(filtered, policy)
		}
		change.Name = state.Hostname
		change.Policies = filtered
		payload, err := json.Marshal(change)
		if err != nil {
			return err
		}
		if _, err := s.master.PublishHTTPJSON(ctx, state.Hostname, payload); err != nil {
			return err
		}
		return deleteChallengeState(s.masterRoot(), http01Key(hostname, token))
	})
}

func (s *HTTP01Solver) masterRoot() string {
	if s.master == nil || s.master.Root() == "" {
		return "."
	}
	return s.master.Root()
}

func http01Key(hostname, token string) string {
	return hostname + "|" + token
}

func challengePath(token string) string {
	return "/.well-known/acme-challenge/" + token
}

func isChallengePolicy(policy master.HTTPPolicyJSON, token string) bool {
	return policy.PathnameKind == "exact" &&
		policy.Pathname == challengePath(token) &&
		policy.FixContent != ""
}

func readHTTPChangeOrEmpty(ctx context.Context, m *master.Master, hostname string) (master.HTTPChangeJSON, error) {
	change, err := m.ReadHTTPJSON(ctx, hostname)
	if err != nil {
		if isNotExist(err) {
			return master.HTTPChangeJSON{Name: hostname}, nil
		}
		return master.HTTPChangeJSON{}, err
	}
	return change, nil
}

func unixNow() int64 {
	return time.Now().Unix()
}
