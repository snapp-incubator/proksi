package main

import "testing"

func TestJSONBytesEqual(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		want    bool
		wantErr bool
	}{
		{"identical objects", `{"a":1,"b":"x"}`, `{"a":1,"b":"x"}`, true, false},
		{"key order ignored", `{"a":1,"b":2}`, `{"b":2,"a":1}`, true, false},
		{"whitespace ignored", `{"a": 1}`, "{\n \"a\":1\n}", true, false},
		{"different value", `{"a":1}`, `{"a":2}`, false, false},
		{"different type", `{"a":1}`, `{"a":"1"}`, false, false},
		{"extra key", `{"a":1}`, `{"a":1,"b":2}`, false, false},
		{"array order matters", `[1,2]`, `[2,1]`, false, false},
		{"nested equal", `{"a":{"b":[1,{"c":null}]}}`, `{"a":{"b":[1,{"c":null}]}}`, true, false},
		{"scalars", `1`, `1`, true, false},
		{"invalid first", `{`, `{}`, false, true},
		{"invalid second", `{}`, `nope`, false, true},
		{"empty input", ``, ``, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JSONBytesEqual([]byte(tt.a), []byte(tt.b))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBodyEqualizerFunc(t *testing.T) {
	tests := []struct {
		name string
		fn   bodyEqualizerFunc
		a, b string
		want bool
	}{
		{"bytes equal", dummyBytesEqual, "hello", "hello", true},
		{"bytes differ", dummyBytesEqual, "hello", "hellO", false},
		{"bytes both empty", dummyBytesEqual, "", "", true},
		{"bytes whitespace significant", dummyBytesEqual, "a", "a ", false},
		{"json via type", JSONBytesEqual, `{"a":1}`, ` {"a" : 1}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn([]byte(tt.a), []byte(tt.b))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
