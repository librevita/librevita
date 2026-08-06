package auth

import (
	"sync"
	"testing"
)

func TestHashPasswordAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned an empty hash")
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword rejected the correct password")
	}

	ok, err = VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword accepted a wrong password")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, hash := range []string{"", "not-a-phc-hash", "$argon2id$v=19$m=8,t=3,p=2$AA$AA", "$argon2i$v=19$m=65536,t=3,p=2$AA$AA"} {
		if _, err := VerifyPassword(hash, "x"); err == nil {
			t.Errorf("VerifyPassword(%q) should fail", hash)
		}
	}
}

func TestHashesAreUnique(t *testing.T) {
	a, err := HashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password must differ (salt)")
	}
}

func TestConcurrentHashesRespectSemaphore(t *testing.T) {
	SetMaxConcurrentHashes(2)
	defer SetMaxConcurrentHashes(defaultMaxConcurrentHashes)

	const workers = 6
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = HashPassword("concurrent-password")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d failed: %v", i, err)
		}
	}
}

func TestSetMaxConcurrentHashesClampsToMinimum(t *testing.T) {
	SetMaxConcurrentHashes(0)
	defer SetMaxConcurrentHashes(defaultMaxConcurrentHashes)

	hash, err := HashPassword("still-works")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if ok, _ := VerifyPassword(hash, "still-works"); !ok {
		t.Fatal("verify failed")
	}
}
