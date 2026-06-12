package history

import "strings"

// ModelPrice is USD per million tokens.
type ModelPrice struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
}

// defaultPrices covers model families with stable, publicly documented
// pricing; entries match by longest prefix against the lowercased model
// name. Anything unknown costs 0 and shows as unpriced — workflows override
// or extend via the pricing config section.
var defaultPrices = map[string]ModelPrice{
	"claude-opus":   {InputPerMTok: 15, OutputPerMTok: 75},
	"claude-sonnet": {InputPerMTok: 3, OutputPerMTok: 15},
	"claude-haiku":  {InputPerMTok: 1, OutputPerMTok: 5},
}

// SetPricing overlays workflow-configured prices onto the defaults. Keys are
// model names or prefixes.
func (s *Store) SetPricing(overrides map[string]ModelPrice) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pricing = make(map[string]ModelPrice, len(defaultPrices)+len(overrides))
	for prefix, price := range defaultPrices {
		s.pricing[prefix] = price
	}
	for prefix, price := range overrides {
		s.pricing[strings.ToLower(strings.TrimSpace(prefix))] = price
	}
}

// Cost returns the USD cost for a run of the given model, or 0 when the
// model is unpriced.
func (s *Store) Cost(model string, tokensIn, tokensOut int64) float64 {
	price, ok := s.priceFor(model)
	if !ok {
		return 0
	}
	return float64(tokensIn)/1e6*price.InputPerMTok + float64(tokensOut)/1e6*price.OutputPerMTok
}

func (s *Store) priceFor(model string) (ModelPrice, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return ModelPrice{}, false
	}

	s.mu.Lock()
	table := s.pricing
	s.mu.Unlock()
	if table == nil {
		table = defaultPrices
	}

	if price, ok := table[model]; ok {
		return price, true
	}

	bestLen := 0
	var best ModelPrice
	for prefix, price := range table {
		if strings.HasPrefix(model, prefix) && len(prefix) > bestLen {
			bestLen = len(prefix)
			best = price
		}
	}
	return best, bestLen > 0
}
