package main

import "testing"

func TestReplaceNames(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		params map[string]int
		want   []string
	}{
		{
			"const1",
			"/simple?v=%(c1)%",
			map[string]int{},
			[]string{"/simple?v=hoge"},
		},
		{
			"vars1",
			"/simple?v=%(v1)%",
			map[string]int{},
			[]string{"/simple?v=v1_0001", "/simple?v=v1_0002"},
		},
		{
			"exvars1",
			"/simple?v=%(ev1)%",
			map[string]int{},
			[]string{"/simple?v=10001"},
		},
		{
			"non vars",
			"/simple",
			map[string]int{},
			[]string{"/simple"},
		},
		{
			"empty expand vars",
			"/simple?v=%()%",
			map[string]int{},
			[]string{"/simple?v=%()%"},
		},
		{
			"non expand vars",
			"/simple?vvv=%(dummyvalue)%",
			map[string]int{},
			[]string{"/simple?vvv=%(dummyvalue)%"},
		},
		{
			"empty path",
			"",
			map[string]int{},
			[]string{""},
		},
		{
			"blank",
			" ",
			map[string]int{},
			[]string{" "},
		},
	}

	config := Config{}
	if err := config.Load("example/vars.yml"); err != nil {
		t.Fatal("fail config loading")
	}
	for _, tt := range cases {
		ret := ReplaceNames(tt.path, map[string]int{})
		ok := false
		for _, w := range tt.want {
			if ret == w {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("%s invalid result: want=%s, ret=%s", tt.name, tt.want, ret)
		}
	}
}
