package tspec

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/cucumber/messages-go/v10"

	"github.com/gruntwork-io/terratest/modules/tspec/formatters"
	"github.com/gruntwork-io/terratest/modules/tspec/internal/models"
)

// TestingT is an interface wrapper around *testing.T
// We need this to use the Terratest methods
// TODO - we can probably remove this.
type TestingT interface {
	Errorf(format string, args ...interface{})
	FailNow()
}

// Scenario represents the executed scenario
type Scenario = messages.Pickle

// Step represents the executed step
type Step = messages.Pickle_PickleStep

// Steps allows to nest steps
// instead of returning an error in step func
// it is possible to return combined steps:
//
//   func multistep(name string) tspec.Steps {
//     return tspec.Steps{
//       fmt.Sprintf(`an user named "%s"`, name),
//       fmt.Sprintf(`user "%s" is authenticated`, name),
//     }
//   }
//
// These steps will be matched and executed in
// sequential order. The first one which fails
// will result in main step failure.
type Steps []string

// StepDefinition is a registered step definition
// contains a StepHandler and regexp which
// is used to match a step. Args which
// were matched by last executed step
//
// This structure is passed to the formatter
// when step is matched and is either failed
// or successful
type StepDefinition = formatters.StepDefinition

// DocString represents the DocString argument made to a step definition
type DocString = messages.PickleStepArgument_PickleDocString

// Table represents the Table argument made to a step definition
type Table = messages.PickleStepArgument_PickleTable

// TestSuiteContext allows various contexts
// to register event handlers.
//
// When running a test suite, the instance of TestSuiteContext
// is passed to all functions (contexts), which
// have it as a first and only argument.
//
// Note that all event hooks does not catch panic errors
// in order to have a trace information
type TestSuiteContext struct {
	beforeSuiteHandlers []func()
	afterSuiteHandlers  []func()
}

// BeforeSuite registers a function or method
// to be run once before suite runner.
//
// Use it to prepare the test suite for a spin.
// Connect and prepare database for instance...
func (ctx *TestSuiteContext) BeforeSuite(fn func()) {
	ctx.beforeSuiteHandlers = append(ctx.beforeSuiteHandlers, fn)
}

// AfterSuite registers a function or method
// to be run once after suite runner
func (ctx *TestSuiteContext) AfterSuite(fn func()) {
	ctx.afterSuiteHandlers = append(ctx.afterSuiteHandlers, fn)
}

// ScenarioContext allows various contexts
// to register steps and event handlers.
//
// When running a scenario, the instance of ScenarioContext
// is passed to all functions (contexts), which
// have it as a first and only argument.
//
// Note that all event hooks does not catch panic errors
// in order to have a trace information. Only step
// executions are catching panic error since it may
// be a context specific error.
type ScenarioContext struct {
	suite *suite
}

// BeforeScenario registers a function or method
// to be run before every scenario.
//
// It is a good practice to restore the default state
// before every scenario so it would be isolated from
// any kind of state.
func (ctx *ScenarioContext) BeforeScenario(fn func(sc *Scenario)) {
	ctx.suite.beforeScenarioHandlers = append(ctx.suite.beforeScenarioHandlers, fn)
}

// AfterScenario registers an function or method
// to be run after every scenario.
func (ctx *ScenarioContext) AfterScenario(fn func(sc *Scenario, err error)) {
	ctx.suite.afterScenarioHandlers = append(ctx.suite.afterScenarioHandlers, fn)
}

// BeforeStep registers a function or method
// to be run before every step.
func (ctx *ScenarioContext) BeforeStep(fn func(st *Step)) {
	ctx.suite.beforeStepHandlers = append(ctx.suite.beforeStepHandlers, fn)
}

// AfterStep registers an function or method
// to be run after every step.
//
// It may be convenient to return a different kind of error
// in order to print more state details which may help
// in case of step failure
//
// In some cases, for example when running a headless
// browser, to take a screenshot after failure.
func (ctx *ScenarioContext) AfterStep(fn func(st *Step, err error)) {
	ctx.suite.afterStepHandlers = append(ctx.suite.afterStepHandlers, fn)
}

// Step allows to register a *StepDefinition in the
// tspec feature suite, the definition will be applied
// to all steps matching the given Regexp expr.
//
// It will panic if expr is not a valid regular
// expression or stepFunc is not a valid step
// handler.
//
// The expression can be of type: *regexp.Regexp, string or []byte
//
// The stepFunc may accept one or several arguments of type:
// - int, int8, int16, int32, int64
// - float32, float64
// - string
// - []byte
// - *tspec.DocString
// - *tspec.Table
//
// The stepFunc need to return either an error or []string for multistep
//
// Note that if there are two definitions which may match
// the same step, then only the first matched handler
// will be applied.
//
// If none of the *StepDefinition is matched, then
// ErrUndefined error will be returned when
// running steps.
func (ctx *ScenarioContext) Step(expr, stepFunc interface{}) {
	var regex *regexp.Regexp

	switch t := expr.(type) {
	case *regexp.Regexp:
		regex = t
	case string:
		regex = regexp.MustCompile(t)
	case []byte:
		regex = regexp.MustCompile(string(t))
	default:
		panic(fmt.Sprintf("expecting expr to be a *regexp.Regexp or a string, got type: %T", expr))
	}

	v := reflect.ValueOf(stepFunc)
	typ := v.Type()
	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("expected handler to be func, but got: %T", stepFunc))
	}

	if typ.NumOut() != 1 {
		panic(fmt.Sprintf("expected handler to return only one value, but it has: %d", typ.NumOut()))
	}

	def := &models.StepDefinition{
		StepDefinition: formatters.StepDefinition{
			Handler: stepFunc,
			Expr:    regex,
		},
		HandlerValue: v,
	}

	typ = typ.Out(0)
	switch typ.Kind() {
	case reflect.Interface:
		if !typ.Implements(errorInterface) {
			panic(fmt.Sprintf("expected handler to return an error, but got: %s", typ.Kind()))
		}
	case reflect.Slice:
		if typ.Elem().Kind() != reflect.String {
			panic(fmt.Sprintf("expected handler to return []string for multistep, but got: []%s", typ.Kind()))
		}
		def.Nested = true
	default:
		panic(fmt.Sprintf("expected handler to return an error or []string, but got: %s", typ.Kind()))
	}

	ctx.suite.steps = append(ctx.suite.steps, def)
}
