package store

import (
	"cmp"
	"slices"

	"github.com/jason-i-magno/career/internal/model"
)

// stableSortByStage orders applications by pipeline progress, preserving the
// caller's secondary ordering (most recently updated) within each stage.
func stableSortByStage(as []model.Application) {
	slices.SortStableFunc(as, func(a, b model.Application) int {
		return cmp.Compare(a.Stage.Order(), b.Stage.Order())
	})
}
