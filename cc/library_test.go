// Copyright 2017 Google Inc. All rights reserved.
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

package cc

import (
	"reflect"
	"testing"

	"android/soong/android"
	"android/soong/bazel/cquery"
)

func TestLibraryReuse(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c", "baz.o"],
		}`)

		libfooShared := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("ld")
		libfooStatic := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_static").Output("libfoo.a")

		if len(libfooShared.Inputs) != 2 {
			t.Fatalf("unexpected inputs to libfoo shared: %#v", libfooShared.Inputs.Strings())
		}

		if len(libfooStatic.Inputs) != 2 {
			t.Fatalf("unexpected inputs to libfoo static: %#v", libfooStatic.Inputs.Strings())
		}

		if libfooShared.Inputs[0] != libfooStatic.Inputs[0] {
			t.Errorf("static object not reused for shared library")
		}
		if libfooShared.Inputs[1] != libfooStatic.Inputs[1] {
			t.Errorf("static object not reused for shared library")
		}
	})

	t.Run("extra static source", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			static: {
				srcs: ["bar.c"]
			},
		}`)

		libfooShared := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("ld")
		libfooStatic := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_static").Output("libfoo.a")

		if len(libfooShared.Inputs) != 1 {
			t.Fatalf("unexpected inputs to libfoo shared: %#v", libfooShared.Inputs.Strings())
		}

		if len(libfooStatic.Inputs) != 2 {
			t.Fatalf("unexpected inputs to libfoo static: %#v", libfooStatic.Inputs.Strings())
		}

		if libfooShared.Inputs[0] != libfooStatic.Inputs[0] {
			t.Errorf("static object not reused for shared library")
		}
	})

	t.Run("extra shared source", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			shared: {
				srcs: ["bar.c"]
			},
		}`)

		libfooShared := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("ld")
		libfooStatic := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_static").Output("libfoo.a")

		if len(libfooShared.Inputs) != 2 {
			t.Fatalf("unexpected inputs to libfoo shared: %#v", libfooShared.Inputs.Strings())
		}

		if len(libfooStatic.Inputs) != 1 {
			t.Fatalf("unexpected inputs to libfoo static: %#v", libfooStatic.Inputs.Strings())
		}

		if libfooShared.Inputs[0] != libfooStatic.Inputs[0] {
			t.Errorf("static object not reused for shared library")
		}
	})

	t.Run("extra static cflags", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			static: {
				cflags: ["-DFOO"],
			},
		}`)

		libfooShared := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("ld")
		libfooStatic := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_static").Output("libfoo.a")

		if len(libfooShared.Inputs) != 1 {
			t.Fatalf("unexpected inputs to libfoo shared: %#v", libfooShared.Inputs.Strings())
		}

		if len(libfooStatic.Inputs) != 1 {
			t.Fatalf("unexpected inputs to libfoo static: %#v", libfooStatic.Inputs.Strings())
		}

		if libfooShared.Inputs[0] == libfooStatic.Inputs[0] {
			t.Errorf("static object reused for shared library when it shouldn't be")
		}
	})

	t.Run("extra shared cflags", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			shared: {
				cflags: ["-DFOO"],
			},
		}`)

		libfooShared := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("ld")
		libfooStatic := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_static").Output("libfoo.a")

		if len(libfooShared.Inputs) != 1 {
			t.Fatalf("unexpected inputs to libfoo shared: %#v", libfooShared.Inputs.Strings())
		}

		if len(libfooStatic.Inputs) != 1 {
			t.Fatalf("unexpected inputs to libfoo static: %#v", libfooStatic.Inputs.Strings())
		}

		if libfooShared.Inputs[0] == libfooStatic.Inputs[0] {
			t.Errorf("static object reused for shared library when it shouldn't be")
		}
	})

	t.Run("global cflags for reused generated sources", func(t *testing.T) {
		ctx := testCc(t, `
		cc_library {
			name: "libfoo",
			srcs: [
				"foo.c",
				"a.proto",
			],
			shared: {
				srcs: [
					"bar.c",
				],
			},
		}`)

		libfooShared := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Rule("ld")
		libfooStatic := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_static").Output("libfoo.a")

		if len(libfooShared.Inputs) != 3 {
			t.Fatalf("unexpected inputs to libfoo shared: %#v", libfooShared.Inputs.Strings())
		}

		if len(libfooStatic.Inputs) != 2 {
			t.Fatalf("unexpected inputs to libfoo static: %#v", libfooStatic.Inputs.Strings())
		}

		if !reflect.DeepEqual(libfooShared.Inputs[0:2].Strings(), libfooStatic.Inputs.Strings()) {
			t.Errorf("static objects not reused for shared library")
		}

		libfoo := ctx.ModuleForTests("libfoo", "android_arm_armv7-a-neon_shared").Module().(*Module)
		if !inList("-DGOOGLE_PROTOBUF_NO_RTTI", libfoo.flags.Local.CFlags) {
			t.Errorf("missing protobuf cflags")
		}
	})
}

func TestStubsVersions(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			stubs: {
				versions: ["29", "R", "current"],
			},
		}
	`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.Platform_version_active_codenames = []string{"R"}
	ctx := testCcWithConfig(t, config)

	variants := ctx.ModuleVariantsForTests("libfoo")
	for _, expectedVer := range []string{"29", "R", "current"} {
		expectedVariant := "android_arm_armv7-a-neon_shared_" + expectedVer
		if !inList(expectedVariant, variants) {
			t.Errorf("missing expected variant: %q", expectedVariant)
		}
	}
}

func TestStubsVersions_NotSorted(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			stubs: {
				versions: ["29", "current", "R"],
			},
		}
	`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.TestProductVariables.Platform_version_active_codenames = []string{"R"}
	testCcErrorWithConfig(t, `"libfoo" .*: versions: not sorted`, config)
}

func TestStubsVersions_ParseError(t *testing.T) {
	bp := `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			stubs: {
				versions: ["29", "current", "X"],
			},
		}
	`

	testCcError(t, `"libfoo" .*: versions: "X" could not be parsed as an integer and is not a recognized codename`, bp)
}

func TestCcLibraryWithBazel(t *testing.T) {
	bp := `
cc_library {
	name: "foo",
	srcs: ["foo.cc"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.BazelContext = android.MockBazelContext{
		OutputBaseDir: "outputbase",
		LabelToCcInfo: map[string]cquery.CcInfo{
			"//foo/bar:bar": cquery.CcInfo{
				CcObjectFiles:        []string{"foo.o"},
				Includes:             []string{"include"},
				SystemIncludes:       []string{"system_include"},
				RootStaticArchives:   []string{"foo.a"},
				RootDynamicLibraries: []string{"foo.so"},
			},
		},
	}
	ctx := testCcWithConfig(t, config)

	staticFoo := ctx.ModuleForTests("foo", "android_arm_armv7-a-neon_static").Module()
	outputFiles, err := staticFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}

	expectedOutputFiles := []string{"outputbase/execroot/__main__/foo.a"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())

	sharedFoo := ctx.ModuleForTests("foo", "android_arm_armv7-a-neon_shared").Module()
	outputFiles, err = sharedFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles = []string{"outputbase/execroot/__main__/foo.so"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())

	entries := android.AndroidMkEntriesForTest(t, ctx, sharedFoo)[0]
	expectedFlags := []string{"-Ioutputbase/execroot/__main__/include", "-isystem outputbase/execroot/__main__/system_include"}
	gotFlags := entries.EntryMap["LOCAL_EXPORT_CFLAGS"]
	android.AssertDeepEquals(t, "androidmk exported cflags", expectedFlags, gotFlags)
}

func TestLibraryVersionScript(t *testing.T) {
	result := PrepareForIntegrationTestWithCc.RunTestWithBp(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			version_script: "foo.map.txt",
		}`)

	libfoo := result.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Rule("ld")

	android.AssertStringListContains(t, "missing dependency on version_script",
		libfoo.Implicits.Strings(), "foo.map.txt")
	android.AssertStringDoesContain(t, "missing flag for version_script",
		libfoo.Args["ldFlags"], "-Wl,--version-script,foo.map.txt")

}

func TestLibraryDynamicList(t *testing.T) {
	result := PrepareForIntegrationTestWithCc.RunTestWithBp(t, `
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			dynamic_list: "foo.dynamic.txt",
		}`)

	libfoo := result.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Rule("ld")

	android.AssertStringListContains(t, "missing dependency on dynamic_list",
		libfoo.Implicits.Strings(), "foo.dynamic.txt")
	android.AssertStringDoesContain(t, "missing flag for dynamic_list",
		libfoo.Args["ldFlags"], "-Wl,--dynamic-list,foo.dynamic.txt")

}

func TestCcLibrarySharedWithBazel(t *testing.T) {
	bp := `
cc_library_shared {
	name: "foo",
	srcs: ["foo.cc"],
	bazel_module: { label: "//foo/bar:bar" },
}`
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	config.BazelContext = android.MockBazelContext{
		OutputBaseDir: "outputbase",
		LabelToCcInfo: map[string]cquery.CcInfo{
			"//foo/bar:bar": cquery.CcInfo{
				CcObjectFiles:        []string{"foo.o"},
				Includes:             []string{"include"},
				SystemIncludes:       []string{"system_include"},
				RootDynamicLibraries: []string{"foo.so"},
			},
		},
	}
	ctx := testCcWithConfig(t, config)

	sharedFoo := ctx.ModuleForTests("foo", "android_arm_armv7-a-neon_shared").Module()
	outputFiles, err := sharedFoo.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Errorf("Unexpected error getting cc_object outputfiles %s", err)
	}
	expectedOutputFiles := []string{"outputbase/execroot/__main__/foo.so"}
	android.AssertDeepEquals(t, "output files", expectedOutputFiles, outputFiles.Strings())

	entries := android.AndroidMkEntriesForTest(t, ctx, sharedFoo)[0]
	expectedFlags := []string{"-Ioutputbase/execroot/__main__/include", "-isystem outputbase/execroot/__main__/system_include"}
	gotFlags := entries.EntryMap["LOCAL_EXPORT_CFLAGS"]
	android.AssertDeepEquals(t, "androidmk exported cflags", expectedFlags, gotFlags)
}
