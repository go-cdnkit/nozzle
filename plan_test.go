package nozzle

import (
	"errors"
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

func TestPlanURLsPreservesDuplicateInputs(t *testing.T) {
	input := []string{"https://example.com/a", "https://example.com/b", "https://example.com/a"}
	plan, err := PlanURLs(input, 2)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("plan is nil")
	}
	want := []Operation{
		{URLs: []string{"https://example.com/a", "https://example.com/b"}},
		{URLs: []string{"https://example.com/a"}},
	}
	if !reflect.DeepEqual(plan.Operations, want) {
		t.Fatalf("operations = %#v, want %#v", plan.Operations, want)
	}
}

func TestPlanURLsRequestLimitBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		urls  []string
		limit int
		sizes []int
	}{
		{name: "below", urls: []string{"https://example.com/a"}, limit: 2, sizes: []int{1}},
		{name: "at", urls: []string{"https://example.com/a", "https://example.com/b"}, limit: 2, sizes: []int{2}},
		{name: "above", urls: []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"}, limit: 2, sizes: []int{2, 1}},
		{name: "exact multiple", urls: []string{"https://example.com/a", "https://example.com/b", "https://example.com/c", "https://example.com/d"}, limit: 2, sizes: []int{2, 2}},
		{name: "one per operation", urls: []string{"https://example.com/a", "https://example.com/b"}, limit: 1, sizes: []int{1, 1}},
		{name: "maximum integer", urls: []string{"https://example.com/a", "https://example.com/b"}, limit: int(^uint(0) >> 1), sizes: []int{2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanURLs(tt.urls, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			if plan == nil {
				t.Fatal("plan is nil")
			}
			var sizes []int
			var flattened []string
			for _, operation := range plan.Operations {
				sizes = append(sizes, len(operation.URLs))
				flattened = append(flattened, operation.URLs...)
			}
			if !reflect.DeepEqual(sizes, tt.sizes) {
				t.Fatalf("operation sizes = %v, want %v", sizes, tt.sizes)
			}
			if !reflect.DeepEqual(flattened, tt.urls) {
				t.Fatalf("planned URLs = %v, want %v", flattened, tt.urls)
			}
		})
	}
}

func TestPlanURLsPreservesExactURLText(t *testing.T) {
	input := []string{
		"https://EXAMPLE.com/Article",
		"https://example.com/a%2fb?b=2&a=1&a=3",
		"https://example.com/a%2Fb?x=a+b&y=a%20b",
		"https://example.com",
		"https://example.com/",
		"https://example.com/path?",
		"http://example.com:8080/path",
		"https://example.com/a/../b",
	}
	plan, err := PlanURLs(input, len(input))
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || len(plan.Operations) != 1 {
		t.Fatalf("plan = %#v, want one operation", plan)
	}
	if !reflect.DeepEqual(plan.Operations[0].URLs, input) {
		t.Fatalf("planned URLs = %q, want %q", plan.Operations[0].URLs, input)
	}
}

func TestPlanURLsRejectsInvalidTargetsAtomically(t *testing.T) {
	invalid := []string{
		"", "/article", "//example.com/article", "ftp://example.com/a",
		"https:///article", "https://", "https://example.com/%zz",
		"https://example.com:bad/a", " https://example.com/a",
		"https://example.com/a b", "https://example.com/a\n",
	}
	for _, target := range invalid {
		t.Run(target, func(t *testing.T) {
			plan, err := PlanURLs([]string{"https://example.com/valid", target, "https://example.com/later"}, 1)
			if err == nil || plan != nil {
				t.Fatalf("plan = %#v, error = %v", plan, err)
			}
			var invalidTarget *InvalidTargetError
			if !errors.As(err, &invalidTarget) || invalidTarget.Index != 1 {
				t.Fatalf("error = %v, want InvalidTargetError at index 1", err)
			}
		})
	}
}

func TestPlanURLsRejectsUserInfo(t *testing.T) {
	for _, target := range []string{"https://user@example.com/a", "https://user:secret@example.com/a"} {
		plan, err := PlanURLs([]string{target}, 1)
		var invalidTarget *InvalidTargetError
		if plan != nil || !errors.As(err, &invalidTarget) || invalidTarget.Index != 0 {
			t.Fatalf("plan = %#v, error = %v", plan, err)
		}
	}
}

func TestPlanURLsRejectsFragments(t *testing.T) {
	for _, target := range []string{"https://example.com/a#section", "https://example.com/a#"} {
		plan, err := PlanURLs([]string{target}, 1)
		var invalidTarget *InvalidTargetError
		if plan != nil || !errors.As(err, &invalidTarget) || invalidTarget.Index != 0 {
			t.Fatalf("plan = %#v, error = %v", plan, err)
		}
	}
}

func TestPlanURLsReportsFirstInvalidInput(t *testing.T) {
	plan, err := PlanURLs([]string{"https://example.com/valid", "/first", "/second"}, 2)
	var invalidTarget *InvalidTargetError
	if plan != nil || !errors.As(err, &invalidTarget) || invalidTarget.Index != 1 {
		t.Fatalf("plan = %#v, error = %v", plan, err)
	}
}

func TestPlanURLsDoesNotAliasCallerData(t *testing.T) {
	input := []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"}
	original := append([]string(nil), input...)
	plan, err := PlanURLs(input, 2)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || len(plan.Operations) != 2 || len(plan.Operations[0].URLs) != 2 || len(plan.Operations[1].URLs) != 1 {
		t.Fatalf("plan = %#v, want operation sizes 2 and 1", plan)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("planning changed input to %v", input)
	}
	input[0] = "https://example.com/caller-change"
	if plan.Operations[0].URLs[0] != original[0] {
		t.Fatal("caller mutation changed the plan")
	}
	plan.Operations[0].URLs[1] = "https://example.com/plan-change"
	if input[1] != original[1] {
		t.Fatal("plan mutation changed the input")
	}
	plan.Operations[0].URLs = append(plan.Operations[0].URLs, "https://example.com/appended")
	if plan.Operations[1].URLs[0] != original[2] || input[2] != original[2] {
		t.Fatal("append changed another operation or the input")
	}
}
