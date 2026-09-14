package nozzle

import (
	"reflect"
	"testing"
)

func TestPlanURLsSingleTarget(t *testing.T) {
	plan, err := PlanURLs([]string{"https://example.com/article"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("plan is nil")
	}
	want := []Operation{{URLs: []string{"https://example.com/article"}}}
	if !reflect.DeepEqual(plan.Operations, want) {
		t.Fatalf("operations = %#v, want %#v", plan.Operations, want)
	}
}

func TestPlanURLsEmptyInput(t *testing.T) {
	for _, input := range [][]string{nil, {}} {
		plan, err := PlanURLs(input, 2)
		if err != nil {
			t.Fatal(err)
		}
		if plan == nil || len(plan.Operations) != 0 {
			t.Fatalf("empty input plan = %#v", plan)
		}
	}
}

func TestPlanURLsRejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		for _, input := range [][]string{nil, {"https://example.com/a"}} {
			plan, err := PlanURLs(input, limit)
			if err == nil || plan != nil {
				t.Fatalf("limit %d, input %v: plan = %#v, error = %v", limit, input, plan, err)
			}
		}
	}
}
