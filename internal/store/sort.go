package store

import (
	"sort"

	"github.com/jason-i-magno/career/internal/model"
)

// stableSortByStage orders applications by pipeline progress, preserving the
// caller's secondary ordering (most recently updated) within each stage.
func stableSortByStage(as []model.Application) {
	sort.SliceStable(as, func(i, j int) bool {
		return as[i].Stage.Order() < as[j].Stage.Order()
	})
}
