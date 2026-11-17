package normalize

import (
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"librevita.org/pkg/errors"
)

//go:embed rules/pt-br.expr
var defaultPtBrScript string

// RuleEnv defines the execution context exposed to normalization scripts.
type RuleEnv struct {
	Word string
}

// RuleEngine executes non-Turing complete Expr normalization scripts using a pool of VMs.
type RuleEngine struct {
	program *vm.Program
	vmPool  sync.Pool
}

// isVowel checks if the first rune of string s is a Latin vowel.
func isVowel(s string) bool {
	if len(s) == 0 {
		return false
	}
	switch s[0] {
	case 'a', 'e', 'i', 'o', 'u', 'y', 'A', 'E', 'I', 'O', 'U', 'Y':
		return true
	default:
		return false
	}
}

// NewRuleEngine compiles an Expr script into bytecode and sets up a VM pool.
func NewRuleEngine(script string) (*RuleEngine, error) {
	script = strings.TrimSpace(script)
	if script == "" {
		return nil, errors.New("normalize: empty rule script")
	}

	program, err := expr.Compile(
		script,
		expr.Env(&RuleEnv{}),
		expr.AsKind(reflect.String),
		expr.Function("stripDiacritics", func(params ...any) (any, error) {
			if len(params) == 0 {
				return "", nil
			}
			s, ok := params[0].(string)
			if !ok {
				return "", fmt.Errorf("stripDiacritics: expected string, got %T", params[0])
			}
			return StripDiacritics(s), nil
		}),
		expr.Function("isVowel", func(params ...any) (any, error) {
			if len(params) == 0 {
				return false, nil
			}
			s, ok := params[0].(string)
			if !ok {
				return false, fmt.Errorf("isVowel: expected string, got %T", params[0])
			}
			return isVowel(s), nil
		}),
		expr.Function("replaceAll", func(params ...any) (any, error) {
			if len(params) < 2 {
				return "", fmt.Errorf("replaceAll: expected (string, map)")
			}
			s, ok := params[0].(string)
			if !ok {
				return "", fmt.Errorf("replaceAll: expected string as first argument, got %T", params[0])
			}
			switch m := params[1].(type) {
			case map[string]any:
				for from, to := range m {
					if toStr, ok := to.(string); ok {
						s = strings.ReplaceAll(s, from, toStr)
					}
				}
			case map[string]string:
				for from, to := range m {
					s = strings.ReplaceAll(s, from, to)
				}
			default:
				return "", fmt.Errorf("replaceAll: expected map for replacements, got %T", params[1])
			}
			return s, nil
		}),
		expr.Function("replaceInitialW", func(params ...any) (any, error) {
			if len(params) == 0 {
				return "", nil
			}
			s, ok := params[0].(string)
			if !ok {
				return "", fmt.Errorf("replaceInitialW: expected string, got %T", params[0])
			}
			if strings.HasPrefix(s, "w") && len(s) > 1 && isVowel(s[1:]) {
				return "v" + s[1:], nil
			}
			return s, nil
		}),
		expr.Function("replacePrefix", func(params ...any) (any, error) {
			if len(params) < 3 {
				return "", fmt.Errorf("replacePrefix: expected (string, prefix, replacement)")
			}
			s, ok := params[0].(string)
			if !ok {
				return "", fmt.Errorf("replacePrefix: expected string, got %T", params[0])
			}
			prefix, ok := params[1].(string)
			if !ok {
				return "", fmt.Errorf("replacePrefix: expected string prefix, got %T", params[1])
			}
			rep, ok := params[2].(string)
			if !ok {
				return "", fmt.Errorf("replacePrefix: expected string replacement, got %T", params[2])
			}
			if strings.HasPrefix(s, prefix) {
				return rep + s[len(prefix):], nil
			}
			return s, nil
		}),
		expr.Function("replaceSuffix", func(params ...any) (any, error) {
			if len(params) < 3 {
				return "", fmt.Errorf("replaceSuffix: expected (string, suffix, replacement)")
			}
			s, ok := params[0].(string)
			if !ok {
				return "", fmt.Errorf("replaceSuffix: expected string, got %T", params[0])
			}
			suffix, ok := params[1].(string)
			if !ok {
				return "", fmt.Errorf("replaceSuffix: expected string suffix, got %T", params[1])
			}
			rep, ok := params[2].(string)
			if !ok {
				return "", fmt.Errorf("replaceSuffix: expected string replacement, got %T", params[2])
			}
			if strings.HasSuffix(s, suffix) {
				return s[:len(s)-len(suffix)] + rep, nil
			}
			return s, nil
		}),
	)
	if err != nil {
		return nil, errors.Wrap(err, "normalize: compile expr script")
	}

	return &RuleEngine{
		program: program,
		vmPool: sync.Pool{
			New: func() any {
				return &vm.VM{}
			},
		},
	}, nil
}

// MustNewRuleEngine compiles an Expr script or panics on failure.
func MustNewRuleEngine(script string) *RuleEngine {
	engine, err := NewRuleEngine(script)
	if err != nil {
		panic(err)
	}
	return engine
}

// LoadRuleEngineFromFile reads an external .expr file and compiles it into a RuleEngine.
func LoadRuleEngineFromFile(filePath string) (*RuleEngine, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "normalize: read rule file %s", filePath)
	}
	return NewRuleEngine(string(content))
}

// Normalize applies the compiled Expr normalization pipeline to a single word.
func (e *RuleEngine) Normalize(word string) string {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return ""
	}

	v, ok := e.vmPool.Get().(*vm.VM)
	if !ok || v == nil {
		v = &vm.VM{}
	}
	defer e.vmPool.Put(v)

	env := RuleEnv{Word: word}
	out, err := v.Run(e.program, &env)
	if err != nil {
		return word
	}

	res, ok := out.(string)
	if !ok {
		return word
	}
	return res
}

var (
	defaultEngineLock sync.RWMutex
	defaultEngine     = MustNewRuleEngine(defaultPtBrScript)
)

// ActiveRuleEngine returns the currently active global RuleEngine.
func ActiveRuleEngine() *RuleEngine {
	defaultEngineLock.RLock()
	defer defaultEngineLock.RUnlock()
	return defaultEngine
}

// SetActiveRuleEngine overrides the active global RuleEngine.
func SetActiveRuleEngine(engine *RuleEngine) {
	if engine == nil {
		return
	}
	defaultEngineLock.Lock()
	defer defaultEngineLock.Unlock()
	defaultEngine = engine
}
