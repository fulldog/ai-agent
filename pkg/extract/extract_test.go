package extract

import "testing"

func TestIsSupportedExtension(t *testing.T) {
	cases := map[string]bool{
		"a.pdf":  true,
		"a.docx": true,
		"a.png":  true,
		"a.jpg":  true,
		"a.doc":  false,
		"a.exe":  false,
	}
	for name, want := range cases {
		if got := IsSupportedExtension(name); got != want {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
}

func TestFromBytesText(t *testing.T) {
	e := New(OCRConfig{Enabled: false})
	text, err := e.FromBytes("note.md", []byte("# hello\nworld"))
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Fatal("empty text")
	}
}
