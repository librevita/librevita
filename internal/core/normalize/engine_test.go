package normalize_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/normalize"
)

func TestRuleEngine_CustomScript(t *testing.T) {
	// A custom script for German names: converting "sch" to "sh" and "w" to "v"
	customScript := `
		let w1 = Word startsWith "sch" ? "sh" + Word[3:] : Word;
		w1 startsWith "w" ? "v" + w1[1:] : w1
	`
	engine, err := normalize.NewRuleEngine(customScript)
	require.NoError(t, err)

	assert.Equal(t, "shmidt", engine.Normalize("schmidt"))
	assert.Equal(t, "valter", engine.Normalize("walter"))
	assert.Equal(t, "mueller", engine.Normalize("mueller"))
}

func TestRuleEngine_BadSyntax(t *testing.T) {
	_, err := normalize.NewRuleEngine("invalid ??? syntax &&&")
	assert.Error(t, err)

	_, err = normalize.NewRuleEngine("")
	assert.Error(t, err)
}

func TestRuleEngine_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "custom.expr")

	script := `Word startsWith "dr" ? Word[2:] : Word`
	err := os.WriteFile(filePath, []byte(script), 0o600)
	require.NoError(t, err)

	engine, err := normalize.LoadRuleEngineFromFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, "john", engine.Normalize("drjohn"))
	assert.Equal(t, "carlos", engine.Normalize("carlos"))
}

func TestRuleEngine_ConcurrentSafety(t *testing.T) {
	engine := normalize.ActiveRuleEngine()
	require.NotNil(t, engine)

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				assert.Equal(t, "valter", engine.Normalize("walter"))
				assert.Equal(t, "luis", engine.Normalize("luiz"))
				assert.Equal(t, "shmit", engine.Normalize("shimit"))
				assert.Equal(t, "felipe", engine.Normalize("phelipe"))
				assert.Equal(t, "vatanabe", engine.Normalize("watanabe"))
			}
		}()
	}

	wg.Wait()
}
