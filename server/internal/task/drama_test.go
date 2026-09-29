package task

import (
	"encoding/json"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"plain", `{"shots":[{"scene":"a"}]}`, `{"shots":[{"scene":"a"}]}`},
		{"fenced", "```json\n{\"shots\":[{\"scene\":\"a\"}]}\n```", `{"shots":[{"scene":"a"}]}`},
		{"prose", "好的，结果如下：\n{\"shots\":[{\"scene\":\"a\"}]}\n希望有帮助。", `{"shots":[{"scene":"a"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSON(tc.content)
			if string(got) != tc.want {
				t.Errorf("extractJSON = %s, want %s", got, tc.want)
			}
		})
	}
	if extractJSON("no json here") != nil {
		t.Error("extractJSON(no json) should be nil")
	}
}

func TestStoryboardJSONRoundTrip(t *testing.T) {
	content := "```json\n" + `{"shots":[{"scene":"雨夜街头","dialogue":"你终于来了","image_prompt":"rainy street at night, cinematic"}]}` + "\n```"
	raw := extractJSON(content)
	if raw == nil {
		t.Fatal("extractJSON returned nil")
	}
	var parsed struct {
		Shots []storyboardShot `json:"shots"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Shots) != 1 || parsed.Shots[0].ImagePrompt == "" {
		t.Errorf("parsed = %+v, want one shot with image_prompt", parsed)
	}
}

func TestNaiveSplit(t *testing.T) {
	script := "第一句。第二句！第三句？\n第四句。"
	// more sentences than requested shots: merged down to 2
	got := naiveSplit(script, "水墨", 2)
	if len(got) != 2 {
		t.Fatalf("naiveSplit len = %d, want 2", len(got))
	}
	if got[0].ImagePrompt != "水墨, 第一句第二句" {
		t.Errorf("shot1 image_prompt = %q", got[0].ImagePrompt)
	}
	// fewer sentences than shots: one shot per sentence, no merge
	got = naiveSplit("一句。两句。", "", 8)
	if len(got) != 2 {
		t.Fatalf("naiveSplit len = %d, want 2", len(got))
	}
	if got[1].Scene != "两句" {
		t.Errorf("shot2 scene = %q", got[1].Scene)
	}
	if naiveSplit("   ", "x", 4) != nil {
		t.Error("naiveSplit(whitespace) should be nil")
	}
}