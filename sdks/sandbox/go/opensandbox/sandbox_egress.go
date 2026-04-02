package opensandbox

import "context"

// GetEgressPolicy retrieves the current egress network policy.
func (s *Sandbox) GetEgressPolicy(ctx context.Context) (*PolicyStatusResponse, error) {
	if err := s.resolveEgress(ctx); err != nil {
		return nil, err
	}
	return s.egress.GetPolicy(ctx)
}

// PatchEgressRules merges network rules into the current egress policy.
func (s *Sandbox) PatchEgressRules(ctx context.Context, rules []NetworkRule) (*PolicyStatusResponse, error) {
	if err := s.resolveEgress(ctx); err != nil {
		return nil, err
	}
	return s.egress.PatchPolicy(ctx, rules)
}
