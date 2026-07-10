package main

import "testing"

func TestPageRefKey(t *testing.T) {
	tests := []struct {
		name string
		ref  pageRef
		want string
	}{
		{"по id", pageRef{ID: "123"}, "id:123"},
		{"по title и space", pageRef{Title: "On-call", Space: "OB"}, "title:OB/On-call"},
		{"id важнее title", pageRef{ID: "123", Title: "On-call", Space: "OB"}, "id:123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.Key(); got != tt.want {
				t.Errorf("Key() = %q, хотим %q", got, tt.want)
			}
		})
	}
}

func TestPageRefIsZero(t *testing.T) {
	if !(pageRef{}).IsZero() {
		t.Error("пустой pageRef должен быть IsZero")
	}
	if (pageRef{ID: "1"}).IsZero() {
		t.Error("pageRef с id не должен быть IsZero")
	}
	if (pageRef{Title: "T", Space: "S"}).IsZero() {
		t.Error("pageRef с title+space не должен быть IsZero")
	}
	if !(pageRef{Title: "T"}).IsZero() {
		t.Error("title без space неадресуем — должен быть IsZero")
	}
}
