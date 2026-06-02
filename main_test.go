package main

import "testing"

func TestParseAddArgs(t *testing.T) {
	type want struct {
		name string
		qty  int
	}
	cases := []struct {
		args []string
		want []want
		err  bool
	}{
		{[]string{"milk"}, []want{{"milk", 1}}, false},
		{[]string{"milk", "2"}, []want{{"milk", 2}}, false},
		{[]string{"milk", "eggs", "bread"}, []want{{"milk", 1}, {"eggs", 1}, {"bread", 1}}, false},
		{[]string{"milk", "2", "eggs", "bread", "3"}, []want{{"milk", 2}, {"eggs", 1}, {"bread", 3}}, false},
		// A trailing qty without an item is operator error — quantities only modify the previous item.
		{[]string{"2"}, nil, true},
		{[]string{"milk", "0"}, nil, true},
		{[]string{}, nil, true},
	}
	for _, tc := range cases {
		got, err := parseAddArgs(tc.args)
		if tc.err {
			if err == nil {
				t.Errorf("parseAddArgs(%v): expected error, got %v", tc.args, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAddArgs(%v): unexpected error %v", tc.args, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseAddArgs(%v): got %d items, want %d", tc.args, len(got), len(tc.want))
			continue
		}
		for i, w := range tc.want {
			if got[i].Name != w.name || got[i].Quantity != w.qty {
				t.Errorf("parseAddArgs(%v)[%d]: got {%q, %d}, want {%q, %d}",
					tc.args, i, got[i].Name, got[i].Quantity, w.name, w.qty)
			}
		}
	}
}
