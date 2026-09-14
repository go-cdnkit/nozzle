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
