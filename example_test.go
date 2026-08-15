package cairn_test

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/devosher01/cairn"
)

func Example() {
	dir, err := os.MkdirTemp("", "cairn-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	db, err := cairn.Open(dir, nil)
	if err != nil {
		panic(err)
	}

	if err := db.Put([]byte("summit"), []byte("2743")); err != nil {
		panic(err)
	}

	value, err := db.Get([]byte("summit"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("summit=%s\n", value)

	if err := db.Delete([]byte("summit")); err != nil {
		panic(err)
	}
	if _, err := db.Get([]byte("summit")); errors.Is(err, cairn.ErrNotFound) {
		fmt.Println("summit=<not found>")
	}

	if err := db.Close(); err != nil {
		panic(err)
	}
	// Output:
	// summit=2743
	// summit=<not found>
}

func ExampleDB_Write() {
	dir, err := os.MkdirTemp("", "cairn-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	db, err := cairn.Open(dir, nil)
	if err != nil {
		panic(err)
	}

	if err := db.Put([]byte("alpha"), []byte("1")); err != nil {
		panic(err)
	}

	b := cairn.NewBatch()
	b.Put([]byte("bravo"), []byte("2"))
	b.Put([]byte("charlie"), []byte("3"))
	b.Delete([]byte("alpha"))
	if err := db.Write(b); err != nil {
		panic(err)
	}

	for _, key := range []string{"alpha", "bravo", "charlie"} {
		value, err := db.Get([]byte(key))
		switch {
		case errors.Is(err, cairn.ErrNotFound):
			fmt.Printf("%s=<not found>\n", key)
		case err != nil:
			panic(err)
		default:
			fmt.Printf("%s=%s\n", key, value)
		}
	}

	if err := db.Close(); err != nil {
		panic(err)
	}
	// Output:
	// alpha=<not found>
	// bravo=2
	// charlie=3
}

func ExampleDB_NewIterator() {
	dir, err := os.MkdirTemp("", "cairn-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	db, err := cairn.Open(dir, nil)
	if err != nil {
		panic(err)
	}

	for _, key := range []string{"echo", "alpha", "delta", "bravo", "charlie"} {
		if err := db.Put([]byte(key), []byte(strings.ToUpper(key))); err != nil {
			panic(err)
		}
	}

	it, err := db.NewIterator(cairn.IterOptions{
		LowerBound: []byte("bravo"),
		UpperBound: []byte("echo"),
	})
	if err != nil {
		panic(err)
	}
	for ok := it.First(); ok; ok = it.Next() {
		fmt.Printf("%s=%s\n", it.Key(), it.Value())
	}
	if err := it.Close(); err != nil {
		panic(err)
	}

	if err := db.Close(); err != nil {
		panic(err)
	}
	// Output:
	// bravo=BRAVO
	// charlie=CHARLIE
	// delta=DELTA
}

func ExampleDB_NewSnapshot() {
	dir, err := os.MkdirTemp("", "cairn-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	db, err := cairn.Open(dir, nil)
	if err != nil {
		panic(err)
	}

	if err := db.Put([]byte("height"), []byte("1")); err != nil {
		panic(err)
	}

	snap, err := db.NewSnapshot()
	if err != nil {
		panic(err)
	}

	if err := db.Put([]byte("height"), []byte("2")); err != nil {
		panic(err)
	}

	live, err := db.Get([]byte("height"))
	if err != nil {
		panic(err)
	}
	pinned, err := snap.Get([]byte("height"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("live=%s snapshot=%s\n", live, pinned)

	if err := snap.Close(); err != nil {
		panic(err)
	}
	if err := db.Close(); err != nil {
		panic(err)
	}
	// Output:
	// live=2 snapshot=1
}
