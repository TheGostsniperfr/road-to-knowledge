package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplit_SharesSlotsByWeight(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		weights []int
		want    []int
	}{
		{"default budget gives the largest remainder to near", 9, []int{60, 30, 10}, []int{5, 3, 1}},
		{"negative weights count as zero", 9, []int{120, 0, -20}, []int{9, 0, 0}},
		{"all zero weights give every slot to near", 4, []int{0, 0, 0}, []int{4, 0, 0}},
		{"all negative weights give every slot to near", 4, []int{-1, -5, -3}, []int{4, 0, 0}},
		{"zero slots stay zero", 0, []int{60, 30, 10}, []int{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := split(tt.n, tt.weights)

			assert.Equal(t, tt.want, got)
		})
	}
}
