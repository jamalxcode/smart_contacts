package main

import (
	"strings"
	"testing"
)

func TestLastChanges(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	a := &Contact{ID: "a", First: "Ali", Last: "Hassan", Phones: []Field{{"Mobile", "65551212"}}}
	b := &Contact{ID: "b", First: "Sara", Company: "Zain"}
	s.Put(a)
	s.Put(b)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	depth := s.UndoDepth()
	a.Phones[0].Value = "+965 6555 1212"
	a.Groups = []string{"Work"}
	s.Put(a)
	s.Delete("b")
	s.Put(&Contact{ID: "c", First: "Omar"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if s.UndoDepth() != depth+1 {
		t.Fatalf("no undo step recorded")
	}
	ch := s.LastChanges()
	if len(ch) != 3 {
		t.Fatalf("want 3 changes, got %+v", ch)
	}
	kinds := ch[0].Kind + "," + ch[1].Kind + "," + ch[2].Kind
	if kinds != "added,changed,deleted" {
		t.Fatalf("order/kinds: %s", kinds)
	}
	var fields []string
	for _, f := range ch[1].Fields {
		fields = append(fields, f.Field)
	}
	if strings.Join(fields, ",") != "Phones,Groups" {
		t.Fatalf("fields: %v", fields)
	}
	if ch[1].Fields[0].Before != "mobile 65551212" || ch[1].Fields[0].After != "mobile +965 6555 1212" {
		t.Fatalf("phone change: %+v", ch[1].Fields[0])
	}
	sum := summariseChanges(ch)
	for _, want := range []string{"1 card added", "Ali Hassan changed: phones, groups", "1 card moved to the Trash"} {
		if !strings.Contains(sum, want) {
			t.Fatalf("summary %q lacks %q", sum, want)
		}
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if u := s.UndoneChanges(); len(u) != 3 {
		t.Fatalf("undo changes: %+v", u)
	}
}
