package trie

import (
	"testing"
)

func TestTrie_InsertSearch(t *testing.T) {
	tr := New()
	tr.Insert("hello", 5)
	tr.Insert("help", 10)
	tr.Insert("helmet", 3)
	tr.Insert("hero", 7)
	tr.Insert("her", 2)

	results := tr.Search("hel", 10)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Most frequent first
	if results[0].Term != "help" || results[0].Freq != 10 {
		t.Errorf("expected help(10) first, got %s(%d)", results[0].Term, results[0].Freq)
	}
	if results[1].Term != "hello" || results[1].Freq != 5 {
		t.Errorf("expected hello(5) second, got %s(%d)", results[1].Term, results[1].Freq)
	}
}

func TestTrie_SearchLimit(t *testing.T) {
	tr := New()
	tr.Insert("a", 1)
	tr.Insert("ab", 2)
	tr.Insert("abc", 3)
	tr.Insert("abcd", 4)

	results := tr.Search("a", 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestTrie_SearchNoMatch(t *testing.T) {
	tr := New()
	tr.Insert("test", 1)
	results := tr.Search("xyz", 10)
	if results != nil && len(results) > 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestTrie_Increment(t *testing.T) {
	tr := New()
	tr.Increment("foo")
	tr.Increment("foo")
	tr.Increment("foo")

	results := tr.Search("f", 10)
	if len(results) != 1 || results[0].Freq != 3 {
		t.Errorf("expected foo(3), got %v", results)
	}
}

func TestTrie_EmptySearch(t *testing.T) {
	tr := New()
	tr.Increment("cat")
	tr.Increment("car")

	results := tr.Search("", 10)
	if len(results) != 2 {
		t.Errorf("expected 2 results for empty prefix, got %d", len(results))
	}
}

func TestTrie_ConcurrentInsert(t *testing.T) {
	tr := New()
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				tr.Increment("key")
			}
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	results := tr.Search("k", 1)
	if len(results) != 1 || results[0].Freq != 1000 {
		t.Errorf("expected key(1000), got %v", results)
	}
}
