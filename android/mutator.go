// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"android/soong/bazel"

	"github.com/google/blueprint"
)

// Phases:
//   run Pre-arch mutators
//   run archMutator
//   run Pre-deps mutators
//   run depsMutator
//   run PostDeps mutators
//   run FinalDeps mutators (CreateVariations disallowed in this phase)
//   continue on to GenerateAndroidBuildActions

// RegisterMutatorsForBazelConversion is a alternate registration pipeline for bp2build. Exported for testing.
func RegisterMutatorsForBazelConversion(ctx *Context, preArchMutators []RegisterMutatorFunc) {
	bp2buildMutators := append(preArchMutators, registerBp2buildConversionMutator)
	registerMutatorsForBazelConversion(ctx, bp2buildMutators)
}

// RegisterMutatorsForApiBazelConversion is an alternate registration pipeline for api_bp2build
// This pipeline restricts generation of Bazel targets to Soong modules that contribute APIs
func RegisterMutatorsForApiBazelConversion(ctx *Context, preArchMutators []RegisterMutatorFunc) {
	bp2buildMutators := append(preArchMutators, registerApiBp2buildConversionMutator)
	registerMutatorsForBazelConversion(ctx, bp2buildMutators)
}

func registerMutatorsForBazelConversion(ctx *Context, bp2buildMutators []RegisterMutatorFunc) {
	mctx := &registerMutatorsContext{
		bazelConversionMode: true,
	}

	allMutators := append([]RegisterMutatorFunc{
		RegisterNamespaceMutator,
		RegisterDefaultsPreArchMutators,
		// TODO(b/165114590): this is required to resolve deps that are only prebuilts, but we should
		// evaluate the impact on conversion.
		RegisterPrebuiltsPreArchMutators,
	},
		bp2buildMutators...)

	// Register bp2build mutators
	for _, f := range allMutators {
		f(mctx)
	}

	mctx.mutators.registerAll(ctx)
}

// collateGloballyRegisteredMutators constructs the list of mutators that have been registered
// with the InitRegistrationContext and will be used at runtime.
func collateGloballyRegisteredMutators() sortableComponents {
	return collateRegisteredMutators(preArch, preDeps, postDeps, finalDeps)
}

// collateRegisteredMutators constructs a single list of mutators from the separate lists.
func collateRegisteredMutators(preArch, preDeps, postDeps, finalDeps []RegisterMutatorFunc) sortableComponents {
	mctx := &registerMutatorsContext{}

	register := func(funcs []RegisterMutatorFunc) {
		for _, f := range funcs {
			f(mctx)
		}
	}

	register(preArch)

	register(preDeps)

	register([]RegisterMutatorFunc{registerDepsMutator})

	register(postDeps)

	mctx.finalPhase = true
	register(finalDeps)

	return mctx.mutators
}

type registerMutatorsContext struct {
	mutators            sortableComponents
	finalPhase          bool
	bazelConversionMode bool
}

type RegisterMutatorsContext interface {
	TopDown(name string, m TopDownMutator) MutatorHandle
	BottomUp(name string, m BottomUpMutator) MutatorHandle
	BottomUpBlueprint(name string, m blueprint.BottomUpMutator) MutatorHandle
	Transition(name string, m TransitionMutator)
}

type RegisterMutatorFunc func(RegisterMutatorsContext)

var preArch = []RegisterMutatorFunc{
	RegisterNamespaceMutator,

	// Check the visibility rules are valid.
	//
	// This must run after the package renamer mutators so that any issues found during
	// validation of the package's default_visibility property are reported using the
	// correct package name and not the synthetic name.
	//
	// This must also be run before defaults mutators as the rules for validation are
	// different before checking the rules than they are afterwards. e.g.
	//    visibility: ["//visibility:private", "//visibility:public"]
	// would be invalid if specified in a module definition but is valid if it results
	// from something like this:
	//
	//    defaults {
	//        name: "defaults",
	//        // Be inaccessible outside a package by default.
	//        visibility: ["//visibility:private"]
	//    }
	//
	//    defaultable_module {
	//        name: "defaultable_module",
	//        defaults: ["defaults"],
	//        // Override the default.
	//        visibility: ["//visibility:public"]
	//    }
	//
	RegisterVisibilityRuleChecker,

	// Record the default_applicable_licenses for each package.
	//
	// This must run before the defaults so that defaults modules can pick up the package default.
	RegisterLicensesPackageMapper,

	// Apply properties from defaults modules to the referencing modules.
	//
	// Any mutators that are added before this will not see any modules created by
	// a DefaultableHook.
	RegisterDefaultsPreArchMutators,

	// Add dependencies on any components so that any component references can be
	// resolved within the deps mutator.
	//
	// Must be run after defaults so it can be used to create dependencies on the
	// component modules that are creating in a DefaultableHook.
	//
	// Must be run before RegisterPrebuiltsPreArchMutators, i.e. before prebuilts are
	// renamed. That is so that if a module creates components using a prebuilt module
	// type that any dependencies (which must use prebuilt_ prefixes) are resolved to
	// the prebuilt module and not the source module.
	RegisterComponentsMutator,

	// Create an association between prebuilt modules and their corresponding source
	// modules (if any).
	//
	// Must be run after defaults mutators to ensure that any modules created by
	// a DefaultableHook can be either a prebuilt or a source module with a matching
	// prebuilt.
	RegisterPrebuiltsPreArchMutators,

	// Gather the licenses properties for all modules for use during expansion and enforcement.
	//
	// This must come after the defaults mutators to ensure that any licenses supplied
	// in a defaults module has been successfully applied before the rules are gathered.
	RegisterLicensesPropertyGatherer,

	// Gather the visibility rules for all modules for us during visibility enforcement.
	//
	// This must come after the defaults mutators to ensure that any visibility supplied
	// in a defaults module has been successfully applied before the rules are gathered.
	RegisterVisibilityRuleGatherer,
}

func registerArchMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUpBlueprint("os", osMutator).Parallel()
	ctx.BottomUp("image", imageMutator).Parallel()
	ctx.BottomUpBlueprint("arch", archMutator).Parallel()
}

var preDeps = []RegisterMutatorFunc{
	registerArchMutator,
}

var postDeps = []RegisterMutatorFunc{
	registerPathDepsMutator,
	RegisterPrebuiltsPostDepsMutators,
	RegisterVisibilityRuleEnforcer,
	RegisterLicensesDependencyChecker,
	registerNeverallowMutator,
	RegisterOverridePostDepsMutators,
}

var finalDeps = []RegisterMutatorFunc{}

func PreArchMutators(f RegisterMutatorFunc) {
	preArch = append(preArch, f)
}

func PreDepsMutators(f RegisterMutatorFunc) {
	preDeps = append(preDeps, f)
}

func PostDepsMutators(f RegisterMutatorFunc) {
	postDeps = append(postDeps, f)
}

func FinalDepsMutators(f RegisterMutatorFunc) {
	finalDeps = append(finalDeps, f)
}

var bp2buildPreArchMutators = []RegisterMutatorFunc{}

// A minimal context for Bp2build conversion
type Bp2buildMutatorContext interface {
	BazelConversionPathContext

	CreateBazelTargetModule(bazel.BazelTargetModuleProperties, CommonAttributes, interface{})
}

// PreArchBp2BuildMutators adds mutators to be register for converting Android Blueprint modules
// into Bazel BUILD targets that should run prior to deps and conversion.
func PreArchBp2BuildMutators(f RegisterMutatorFunc) {
	bp2buildPreArchMutators = append(bp2buildPreArchMutators, f)
}

type BaseMutatorContext interface {
	BaseModuleContext

	// MutatorName returns the name that this mutator was registered with.
	MutatorName() string

	// Rename all variants of a module.  The new name is not visible to calls to ModuleName,
	// AddDependency or OtherModuleName until after this mutator pass is complete.
	Rename(name string)
}

type TopDownMutator func(TopDownMutatorContext)

type TopDownMutatorContext interface {
	BaseMutatorContext

	// CreateModule creates a new module by calling the factory method for the specified moduleType, and applies
	// the specified property structs to it as if the properties were set in a blueprint file.
	CreateModule(ModuleFactory, ...interface{}) Module

	// CreateBazelTargetModule creates a BazelTargetModule by calling the
	// factory method, just like in CreateModule, but also requires
	// BazelTargetModuleProperties containing additional metadata for the
	// bp2build codegenerator.
	CreateBazelTargetModule(bazel.BazelTargetModuleProperties, CommonAttributes, interface{})

	// CreateBazelTargetModuleWithRestrictions creates a BazelTargetModule by calling the
	// factory method, just like in CreateModule, but also requires
	// BazelTargetModuleProperties containing additional metadata for the
	// bp2build codegenerator. The generated target is restricted to only be buildable for certain
	// platforms, as dictated by a given bool attribute: the target will not be buildable in
	// any platform for which this bool attribute is false.
	CreateBazelTargetModuleWithRestrictions(bazel.BazelTargetModuleProperties, CommonAttributes, interface{}, bazel.BoolAttribute)
}

type topDownMutatorContext struct {
	bp blueprint.TopDownMutatorContext
	baseModuleContext
}

type BottomUpMutator func(BottomUpMutatorContext)

type BottomUpMutatorContext interface {
	BaseMutatorContext

	// AddDependency adds a dependency to the given module.  It returns a slice of modules for each
	// dependency (some entries may be nil).
	//
	// If the mutator is parallel (see MutatorHandle.Parallel), this method will pause until the
	// new dependencies have had the current mutator called on them.  If the mutator is not
	// parallel this method does not affect the ordering of the current mutator pass, but will
	// be ordered correctly for all future mutator passes.
	AddDependency(module blueprint.Module, tag blueprint.DependencyTag, name ...string) []blueprint.Module

	// AddReverseDependency adds a dependency from the destination to the given module.
	// Does not affect the ordering of the current mutator pass, but will be ordered
	// correctly for all future mutator passes.  All reverse dependencies for a destination module are
	// collected until the end of the mutator pass, sorted by name, and then appended to the destination
	// module's dependency list.
	AddReverseDependency(module blueprint.Module, tag blueprint.DependencyTag, name string)

	// CreateVariations splits  a module into multiple variants, one for each name in the variationNames
	// parameter.  It returns a list of new modules in the same order as the variationNames
	// list.
	//
	// If any of the dependencies of the module being operated on were already split
	// by calling CreateVariations with the same name, the dependency will automatically
	// be updated to point the matching variant.
	//
	// If a module is split, and then a module depending on the first module is not split
	// when the Mutator is later called on it, the dependency of the depending module will
	// automatically be updated to point to the first variant.
	CreateVariations(...string) []Module

	// CreateLocationVariations splits a module into multiple variants, one for each name in the variantNames
	// parameter.  It returns a list of new modules in the same order as the variantNames
	// list.
	//
	// Local variations do not affect automatic dependency resolution - dependencies added
	// to the split module via deps or DynamicDependerModule must exactly match a variant
	// that contains all the non-local variations.
	CreateLocalVariations(...string) []Module

	// SetDependencyVariation sets all dangling dependencies on the current module to point to the variation
	// with given name. This function ignores the default variation set by SetDefaultDependencyVariation.
	SetDependencyVariation(string)

	// SetDefaultDependencyVariation sets the default variation when a dangling reference is detected
	// during the subsequent calls on Create*Variations* functions. To reset, set it to nil.
	SetDefaultDependencyVariation(*string)

	// AddVariationDependencies adds deps as dependencies of the current module, but uses the variations
	// argument to select which variant of the dependency to use.  It returns a slice of modules for
	// each dependency (some entries may be nil).  A variant of the dependency must exist that matches
	// all the non-local variations of the current module, plus the variations argument.
	//
	// If the mutator is parallel (see MutatorHandle.Parallel), this method will pause until the
	// new dependencies have had the current mutator called on them.  If the mutator is not
	// parallel this method does not affect the ordering of the current mutator pass, but will
	// be ordered correctly for all future mutator passes.
	AddVariationDependencies(variations []blueprint.Variation, tag blueprint.DependencyTag, names ...string) []blueprint.Module

	// AddFarVariationDependencies adds deps as dependencies of the current module, but uses the
	// variations argument to select which variant of the dependency to use.  It returns a slice of
	// modules for each dependency (some entries may be nil).  A variant of the dependency must
	// exist that matches the variations argument, but may also have other variations.
	// For any unspecified variation the first variant will be used.
	//
	// Unlike AddVariationDependencies, the variations of the current module are ignored - the
	// dependency only needs to match the supplied variations.
	//
	// If the mutator is parallel (see MutatorHandle.Parallel), this method will pause until the
	// new dependencies have had the current mutator called on them.  If the mutator is not
	// parallel this method does not affect the ordering of the current mutator pass, but will
	// be ordered correctly for all future mutator passes.
	AddFarVariationDependencies([]blueprint.Variation, blueprint.DependencyTag, ...string) []blueprint.Module

	// AddInterVariantDependency adds a dependency between two variants of the same module.  Variants are always
	// ordered in the same orderas they were listed in CreateVariations, and AddInterVariantDependency does not change
	// that ordering, but it associates a DependencyTag with the dependency and makes it visible to VisitDirectDeps,
	// WalkDeps, etc.
	AddInterVariantDependency(tag blueprint.DependencyTag, from, to blueprint.Module)

	// ReplaceDependencies replaces all dependencies on the identical variant of the module with the
	// specified name with the current variant of this module.  Replacements don't take effect until
	// after the mutator pass is finished.
	ReplaceDependencies(string)

	// ReplaceDependencies replaces all dependencies on the identical variant of the module with the
	// specified name with the current variant of this module as long as the supplied predicate returns
	// true.
	//
	// Replacements don't take effect until after the mutator pass is finished.
	ReplaceDependenciesIf(string, blueprint.ReplaceDependencyPredicate)

	// AliasVariation takes a variationName that was passed to CreateVariations for this module,
	// and creates an alias from the current variant (before the mutator has run) to the new
	// variant.  The alias will be valid until the next time a mutator calls CreateVariations or
	// CreateLocalVariations on this module without also calling AliasVariation.  The alias can
	// be used to add dependencies on the newly created variant using the variant map from
	// before CreateVariations was run.
	AliasVariation(variationName string)

	// CreateAliasVariation takes a toVariationName that was passed to CreateVariations for this
	// module, and creates an alias from a new fromVariationName variant the toVariationName
	// variant.  The alias will be valid until the next time a mutator calls CreateVariations or
	// CreateLocalVariations on this module without also calling AliasVariation.  The alias can
	// be used to add dependencies on the toVariationName variant using the fromVariationName
	// variant.
	CreateAliasVariation(fromVariationName, toVariationName string)

	// SetVariationProvider sets the value for a provider for the given newly created variant of
	// the current module, i.e. one of the Modules returned by CreateVariations..  It panics if
	// not called during the appropriate mutator or GenerateBuildActions pass for the provider,
	// if the value is not of the appropriate type, or if the module is not a newly created
	// variant of the current module.  The value should not be modified after being passed to
	// SetVariationProvider.
	SetVariationProvider(module blueprint.Module, provider blueprint.ProviderKey, value interface{})
}

type bottomUpMutatorContext struct {
	bp blueprint.BottomUpMutatorContext
	baseModuleContext
	finalPhase bool
}

func bottomUpMutatorContextFactory(ctx blueprint.BottomUpMutatorContext, a Module,
	finalPhase, bazelConversionMode bool) BottomUpMutatorContext {

	moduleContext := a.base().baseModuleContextFactory(ctx)
	moduleContext.bazelConversionMode = bazelConversionMode

	return &bottomUpMutatorContext{
		bp:                ctx,
		baseModuleContext: a.base().baseModuleContextFactory(ctx),
		finalPhase:        finalPhase,
	}
}

func (x *registerMutatorsContext) BottomUp(name string, m BottomUpMutator) MutatorHandle {
	finalPhase := x.finalPhase
	bazelConversionMode := x.bazelConversionMode
	f := func(ctx blueprint.BottomUpMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			m(bottomUpMutatorContextFactory(ctx, a, finalPhase, bazelConversionMode))
		}
	}
	mutator := &mutator{name: x.mutatorName(name), bottomUpMutator: f}
	x.mutators = append(x.mutators, mutator)
	return mutator
}

func (x *registerMutatorsContext) BottomUpBlueprint(name string, m blueprint.BottomUpMutator) MutatorHandle {
	mutator := &mutator{name: name, bottomUpMutator: m}
	x.mutators = append(x.mutators, mutator)
	return mutator
}

type IncomingTransitionContext interface {
	// Module returns the target of the dependency edge for which the transition
	// is being computed
	Module() Module

	// Config returns the configuration for the build.
	Config() Config
}

type OutgoingTransitionContext interface {
	// Module returns the target of the dependency edge for which the transition
	// is being computed
	Module() Module

	// DepTag() Returns the dependency tag through which this dependency is
	// reached
	DepTag() blueprint.DependencyTag
}

// Transition mutators implement a top-down mechanism where a module tells its
// direct dependencies what variation they should be built in but the dependency
// has the final say.
//
// When implementing a transition mutator, one needs to implement four methods:
//   - Split() that tells what variations a module has by itself
//   - OutgoingTransition() where a module tells what it wants from its
//     dependency
//   - IncomingTransition() where a module has the final say about its own
//     variation
//   - Mutate() that changes the state of a module depending on its variation
//
// That the effective variation of module B when depended on by module A is the
// composition the outgoing transition of module A and the incoming transition
// of module B.
//
// the outgoing transition should not take the properties of the dependency into
// account, only those of the module that depends on it. For this reason, the
// dependency is not even passed into it as an argument. Likewise, the incoming
// transition should not take the properties of the depending module into
// account and is thus not informed about it. This makes for a nice
// decomposition of the decision logic.
//
// A given transition mutator only affects its own variation; other variations
// stay unchanged along the dependency edges.
//
// Soong makes sure that all modules are created in the desired variations and
// that dependency edges are set up correctly. This ensures that "missing
// variation" errors do not happen and allows for more flexible changes in the
// value of the variation among dependency edges (as oppposed to bottom-up
// mutators where if module A in variation X depends on module B and module B
// has that variation X, A must depend on variation X of B)
//
// The limited power of the context objects passed to individual mutators
// methods also makes it more difficult to shoot oneself in the foot. Complete
// safety is not guaranteed because no one prevents individual transition
// mutators from mutating modules in illegal ways and for e.g. Split() or
// Mutate() to run their own visitations of the transitive dependency of the
// module and both of these are bad ideas, but it's better than no guardrails at
// all.
//
// This model is pretty close to Bazel's configuration transitions. The mapping
// between concepts in Soong and Bazel is as follows:
//   - Module == configured target
//   - Variant == configuration
//   - Variation name == configuration flag
//   - Variation == configuration flag value
//   - Outgoing transition == attribute transition
//   - Incoming transition == rule transition
//
// The Split() method does not have a Bazel equivalent and Bazel split
// transitions do not have a Soong equivalent.
//
// Mutate() does not make sense in Bazel due to the different models of the
// two systems: when creating new variations, Soong clones the old module and
// thus some way is needed to change it state whereas Bazel creates each
// configuration of a given configured target anew.
type TransitionMutator interface {
	// Split returns the set of variations that should be created for a module no
	// matter who depends on it. Used when Make depends on a particular variation
	// or when the module knows its variations just based on information given to
	// it in the Blueprint file. This method should not mutate the module it is
	// called on.
	Split(ctx BaseModuleContext) []string

	// Called on a module to determine which variation it wants from its direct
	// dependencies. The dependency itself can override this decision. This method
	// should not mutate the module itself.
	OutgoingTransition(ctx OutgoingTransitionContext, sourceVariation string) string

	// Called on a module to determine which variation it should be in based on
	// the variation modules that depend on it want. This gives the module a final
	// say about its own variations. This method should not mutate the module
	// itself.
	IncomingTransition(ctx IncomingTransitionContext, incomingVariation string) string

	// Called after a module was split into multiple variations on each variation.
	// It should not split the module any further but adding new dependencies is
	// fine. Unlike all the other methods on TransitionMutator, this method is
	// allowed to mutate the module.
	Mutate(ctx BottomUpMutatorContext, variation string)
}

type androidTransitionMutator struct {
	finalPhase          bool
	bazelConversionMode bool
	mutator             TransitionMutator
}

func (a *androidTransitionMutator) Split(ctx blueprint.BaseModuleContext) []string {
	if m, ok := ctx.Module().(Module); ok {
		moduleContext := m.base().baseModuleContextFactory(ctx)
		moduleContext.bazelConversionMode = a.bazelConversionMode
		return a.mutator.Split(&moduleContext)
	} else {
		return []string{""}
	}
}

type outgoingTransitionContextImpl struct {
	bp blueprint.OutgoingTransitionContext
}

func (c *outgoingTransitionContextImpl) Module() Module {
	return c.bp.Module().(Module)
}

func (c *outgoingTransitionContextImpl) DepTag() blueprint.DependencyTag {
	return c.bp.DepTag()
}

func (a *androidTransitionMutator) OutgoingTransition(ctx blueprint.OutgoingTransitionContext, sourceVariation string) string {
	if _, ok := ctx.Module().(Module); ok {
		return a.mutator.OutgoingTransition(&outgoingTransitionContextImpl{bp: ctx}, sourceVariation)
	} else {
		return ""
	}
}

type incomingTransitionContextImpl struct {
	bp blueprint.IncomingTransitionContext
}

func (c *incomingTransitionContextImpl) Module() Module {
	return c.bp.Module().(Module)
}

func (c *incomingTransitionContextImpl) Config() Config {
	return c.bp.Config().(Config)
}

func (a *androidTransitionMutator) IncomingTransition(ctx blueprint.IncomingTransitionContext, incomingVariation string) string {
	if _, ok := ctx.Module().(Module); ok {
		return a.mutator.IncomingTransition(&incomingTransitionContextImpl{bp: ctx}, incomingVariation)
	} else {
		return ""
	}
}

func (a *androidTransitionMutator) Mutate(ctx blueprint.BottomUpMutatorContext, variation string) {
	if am, ok := ctx.Module().(Module); ok {
		a.mutator.Mutate(bottomUpMutatorContextFactory(ctx, am, a.finalPhase, a.bazelConversionMode), variation)
	}
}

func (x *registerMutatorsContext) Transition(name string, m TransitionMutator) {
	atm := &androidTransitionMutator{
		finalPhase:          x.finalPhase,
		bazelConversionMode: x.bazelConversionMode,
		mutator:             m,
	}
	mutator := &mutator{
		name:              name,
		transitionMutator: atm}
	x.mutators = append(x.mutators, mutator)
}

func (x *registerMutatorsContext) mutatorName(name string) string {
	if x.bazelConversionMode {
		return name + "_bp2build"
	}
	return name
}

func (x *registerMutatorsContext) TopDown(name string, m TopDownMutator) MutatorHandle {
	f := func(ctx blueprint.TopDownMutatorContext) {
		if a, ok := ctx.Module().(Module); ok {
			moduleContext := a.base().baseModuleContextFactory(ctx)
			moduleContext.bazelConversionMode = x.bazelConversionMode
			actx := &topDownMutatorContext{
				bp:                ctx,
				baseModuleContext: moduleContext,
			}
			m(actx)
		}
	}
	mutator := &mutator{name: x.mutatorName(name), topDownMutator: f}
	x.mutators = append(x.mutators, mutator)
	return mutator
}

func (mutator *mutator) componentName() string {
	return mutator.name
}

func (mutator *mutator) register(ctx *Context) {
	blueprintCtx := ctx.Context
	var handle blueprint.MutatorHandle
	if mutator.bottomUpMutator != nil {
		handle = blueprintCtx.RegisterBottomUpMutator(mutator.name, mutator.bottomUpMutator)
	} else if mutator.topDownMutator != nil {
		handle = blueprintCtx.RegisterTopDownMutator(mutator.name, mutator.topDownMutator)
	} else if mutator.transitionMutator != nil {
		blueprintCtx.RegisterTransitionMutator(mutator.name, mutator.transitionMutator)
	}
	if mutator.parallel {
		handle.Parallel()
	}
}

type MutatorHandle interface {
	Parallel() MutatorHandle
}

func (mutator *mutator) Parallel() MutatorHandle {
	mutator.parallel = true
	return mutator
}

func RegisterComponentsMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("component-deps", componentDepsMutator).Parallel()
}

// A special mutator that runs just prior to the deps mutator to allow the dependencies
// on component modules to be added so that they can depend directly on a prebuilt
// module.
func componentDepsMutator(ctx BottomUpMutatorContext) {
	if m := ctx.Module(); m.Enabled() {
		m.ComponentDepsMutator(ctx)
	}
}

func depsMutator(ctx BottomUpMutatorContext) {
	if m := ctx.Module(); m.Enabled() {
		m.DepsMutator(ctx)
	}
}

func registerDepsMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("deps", depsMutator).Parallel()
}

func registerDepsMutatorBp2Build(ctx RegisterMutatorsContext) {
	// TODO(b/179313531): Consider a separate mutator that only runs depsMutator for modules that are
	// being converted to build targets.
	ctx.BottomUp("deps", depsMutator).Parallel()
}

func (t *topDownMutatorContext) CreateBazelTargetModule(
	bazelProps bazel.BazelTargetModuleProperties,
	commonAttrs CommonAttributes,
	attrs interface{}) {
	t.createBazelTargetModule(bazelProps, commonAttrs, attrs, bazel.BoolAttribute{})
}

func (t *topDownMutatorContext) CreateBazelTargetModuleWithRestrictions(
	bazelProps bazel.BazelTargetModuleProperties,
	commonAttrs CommonAttributes,
	attrs interface{},
	enabledProperty bazel.BoolAttribute) {
	t.createBazelTargetModule(bazelProps, commonAttrs, attrs, enabledProperty)
}

func (t *topDownMutatorContext) createBazelTargetModule(
	bazelProps bazel.BazelTargetModuleProperties,
	commonAttrs CommonAttributes,
	attrs interface{},
	enabledProperty bazel.BoolAttribute) {
	constraintAttributes := commonAttrs.fillCommonBp2BuildModuleAttrs(t, enabledProperty)
	mod := t.Module()
	info := bp2buildInfo{
		Dir:             t.OtherModuleDir(mod),
		BazelProps:      bazelProps,
		CommonAttrs:     commonAttrs,
		ConstraintAttrs: constraintAttributes,
		Attrs:           attrs,
	}
	mod.base().addBp2buildInfo(info)
}

// android.topDownMutatorContext either has to embed blueprint.TopDownMutatorContext, in which case every method that
// has an overridden version in android.BaseModuleContext has to be manually forwarded to BaseModuleContext to avoid
// ambiguous method errors, or it has to store a blueprint.TopDownMutatorContext non-embedded, in which case every
// non-overridden method has to be forwarded.  There are fewer non-overridden methods, so use the latter.  The following
// methods forward to the identical blueprint versions for topDownMutatorContext and bottomUpMutatorContext.

func (t *topDownMutatorContext) MutatorName() string {
	return t.bp.MutatorName()
}

func (t *topDownMutatorContext) Rename(name string) {
	t.bp.Rename(name)
	t.Module().base().commonProperties.DebugName = name
}

func (t *topDownMutatorContext) createModule(factory blueprint.ModuleFactory, name string, props ...interface{}) blueprint.Module {
	return t.bp.CreateModule(factory, name, props...)
}

func (t *topDownMutatorContext) CreateModule(factory ModuleFactory, props ...interface{}) Module {
	return createModule(t, factory, "_topDownMutatorModule", props...)
}

func (t *topDownMutatorContext) createModuleWithoutInheritance(factory ModuleFactory, props ...interface{}) Module {
	module := t.bp.CreateModule(ModuleFactoryAdaptor(factory), "", props...).(Module)
	return module
}

func (b *bottomUpMutatorContext) MutatorName() string {
	return b.bp.MutatorName()
}

func (b *bottomUpMutatorContext) Rename(name string) {
	b.bp.Rename(name)
	b.Module().base().commonProperties.DebugName = name
}

func (b *bottomUpMutatorContext) AddDependency(module blueprint.Module, tag blueprint.DependencyTag, name ...string) []blueprint.Module {
	return b.bp.AddDependency(module, tag, name...)
}

func (b *bottomUpMutatorContext) AddReverseDependency(module blueprint.Module, tag blueprint.DependencyTag, name string) {
	b.bp.AddReverseDependency(module, tag, name)
}

func (b *bottomUpMutatorContext) CreateVariations(variations ...string) []Module {
	if b.finalPhase {
		panic("CreateVariations not allowed in FinalDepsMutators")
	}

	modules := b.bp.CreateVariations(variations...)

	aModules := make([]Module, len(modules))
	for i := range variations {
		aModules[i] = modules[i].(Module)
		base := aModules[i].base()
		base.commonProperties.DebugMutators = append(base.commonProperties.DebugMutators, b.MutatorName())
		base.commonProperties.DebugVariations = append(base.commonProperties.DebugVariations, variations[i])
	}

	return aModules
}

func (b *bottomUpMutatorContext) CreateLocalVariations(variations ...string) []Module {
	if b.finalPhase {
		panic("CreateLocalVariations not allowed in FinalDepsMutators")
	}

	modules := b.bp.CreateLocalVariations(variations...)

	aModules := make([]Module, len(modules))
	for i := range variations {
		aModules[i] = modules[i].(Module)
		base := aModules[i].base()
		base.commonProperties.DebugMutators = append(base.commonProperties.DebugMutators, b.MutatorName())
		base.commonProperties.DebugVariations = append(base.commonProperties.DebugVariations, variations[i])
	}

	return aModules
}

func (b *bottomUpMutatorContext) SetDependencyVariation(variation string) {
	b.bp.SetDependencyVariation(variation)
}

func (b *bottomUpMutatorContext) SetDefaultDependencyVariation(variation *string) {
	b.bp.SetDefaultDependencyVariation(variation)
}

func (b *bottomUpMutatorContext) AddVariationDependencies(variations []blueprint.Variation, tag blueprint.DependencyTag,
	names ...string) []blueprint.Module {
	return b.bp.AddVariationDependencies(variations, tag, names...)
}

func (b *bottomUpMutatorContext) AddFarVariationDependencies(variations []blueprint.Variation,
	tag blueprint.DependencyTag, names ...string) []blueprint.Module {

	return b.bp.AddFarVariationDependencies(variations, tag, names...)
}

func (b *bottomUpMutatorContext) AddInterVariantDependency(tag blueprint.DependencyTag, from, to blueprint.Module) {
	b.bp.AddInterVariantDependency(tag, from, to)
}

func (b *bottomUpMutatorContext) ReplaceDependencies(name string) {
	b.bp.ReplaceDependencies(name)
}

func (b *bottomUpMutatorContext) ReplaceDependenciesIf(name string, predicate blueprint.ReplaceDependencyPredicate) {
	b.bp.ReplaceDependenciesIf(name, predicate)
}

func (b *bottomUpMutatorContext) AliasVariation(variationName string) {
	b.bp.AliasVariation(variationName)
}

func (b *bottomUpMutatorContext) CreateAliasVariation(fromVariationName, toVariationName string) {
	b.bp.CreateAliasVariation(fromVariationName, toVariationName)
}

func (b *bottomUpMutatorContext) SetVariationProvider(module blueprint.Module, provider blueprint.ProviderKey, value interface{}) {
	b.bp.SetVariationProvider(module, provider, value)
}