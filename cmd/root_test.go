package cmd

import "testing"

func TestCSSFlagIsRepeatable(t *testing.T) {
	flag := rootCmd.Flags().Lookup("css")
	if flag == nil {
		t.Fatal("expected --css flag to be registered on root command")
	}
	if got := flag.Value.Type(); got != "stringArray" {
		t.Fatalf("expected --css to be a repeatable stringArray flag, got %q", got)
	}

	if err := rootCmd.ParseFlags([]string{"--css", "a.css", "--css", "b.css"}); err != nil {
		t.Fatalf("parse --css flags: %v", err)
	}
	got, err := rootCmd.Flags().GetStringArray("css")
	if err != nil {
		t.Fatalf("read --css flag: %v", err)
	}
	want := []string{"a.css", "b.css"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("css[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
