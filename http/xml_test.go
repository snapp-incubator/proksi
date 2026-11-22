package main

import "testing"

func TestXMLBytesEqual(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		want    bool
		wantErr bool
	}{
		{"identical", `<a><b>1</b></a>`, `<a><b>1</b></a>`, true, false},
		{"whitespace", "<a>\n  <b>1</b>\n</a>", `<a><b>1</b></a>`, true, false},
		{"attr order", `<a x="1" y="2"/>`, `<a y="2" x="1"/>`, true, false},
		{"comments and prolog", `<?xml version="1.0"?><!-- c --><a/>`, `<a></a>`, true, false},
		{"text differs", `<a>1</a>`, `<a>2</a>`, false, false},
		{"attr differs", `<a x="1"/>`, `<a x="2"/>`, false, false},
		{"element differs", `<a><b/></a>`, `<a><c/></a>`, false, false},
		{"child order differs", `<a><b/><c/></a>`, `<a><c/><b/></a>`, false, false},
		{"namespace differs", `<a xmlns="urn:x"/>`, `<a xmlns="urn:y"/>`, false, false},
		{"invalid", `<a>`, `<a/>`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xmlBytesEqual([]byte(tt.a), []byte(tt.b))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
