# VetBot

VetBot is responsible for finding instances in which references to rangeloop variables *may* be improperly used.

## Overview

VetBot starts from a list of GitHub repositories to read from. It reads the default branch in each repository as a tarball, parsing any `.go` files it finds. Once it's built a parse tree for all the code in the repository, it runs two static analyzers tailored to the rangeloop capture problem. If either of these analyzers report an issue for a section of code, VetBot opens an issue on [a specific repository](https://github.com/github-vet/rangeloop-pointer-findings) which contains the segment of code that triggered the analyzer, and a link back to the repository where the code was found.

The high-level workflow can be summarized as:

1) Parse Repository
1) Run Static Analysis
1) Report Findings

Each step is explained in more detail below.

## 1. Parse Repository

VetBot's main loop visits new GitHub repositories as long as one is available.
VetBot starts by reading a master list of GitHub repositories into memory from a file. It then samples uniformly from the set of unvisited repositories, parses the code in each repository in its entirety, and runs the static analysis. Once a repository has been parsed successfully, VetBot tracks its completion in a separate file.

## 3. Report Findings

When static analysis reports a finding VetBot then decides if is a duplicate and, if not, opens a new GitHub issue. VetBot the MD5 hash of the source code snippet to detect and discard duplicate findings. VetBot records the GitHub repository where its issues are opened as well as the MD5 hash of all of its findings.

## 2. Run Static Analysis

Between parsing the repository and reporting findings, VetBot runs the static analysis. Go provides strong support for static analysis by making [the parser](https://pkg.go.dev/go/parser) and a [static analysis interface](https://pkg.go.dev/golang.org/x/tools/go/analysis) available as part of its standard library.

While VetBot contains several Analyzers, most only play a supporting role for the two primary analyzers, `loopclosure` and `looppointer`, both of which are variants of open-source Analyzers found online. [`loopclosure`](https://github.com/golang/tools/blob/master/go/analysis/passes/loopclosure/loopclosure.go) is part of the Go standard library, and runs as part of the `go vet` command. [`looppointer`](https://github.com/kyoh86/looppointer) is an MIT-licensed linter originally authored by [Kyoichiro Yamada](https://github.com/kyoh86).

### `loopclosure`

Loopclosure reports on range loop variables which are used inside an anonymous function started via a `go` or `defer` statement.
The version of loopclosure used in VetBot is modified to handle nested block statements and remove its dependence on the type-checker.

Thanks to [Daniel Chatfield](https://www.danielchatfield.com/) for sharing his own loopclosure variant on request, which served as inspiration for several ideas in VetBot.

### `looppointer`

Looppointer reports on any unary reference expression that refers to variables defined on the left-hand side of a `range` expression. It was originally intended for use as a linter, and was designed to be as sensitive as possible. As a result, running looppointer on every Go repository on GitHub results in a large number of false-positives (anecdotally, 500 hits per hour) -- far too many to expect the community to examine via crowdsourcing. To mitigate them, the version of looppointer used in VetBot was augmented to run 4 additional analysis passes.

1. `callgraph` computes an approximate callgraph from the repository.
1. `packid` performs name resolution for package identifiers, to avoid false-positives due to incomplete information in the approximate callgraph.
1. `nogofunc` uses the approximate callgraph to find functions it can prove do not start any goroutines.
1. `pointerescapes` uses the approximate callgraph to find functions it can prove do not store pointers passed to it.

### `callgraph`

The `callgraph` analyzer computes an approximate [call graph](https://en.wikipedia.org/wiki/Call_graph) based only on syntactic information. The call graph produced includes only the name of each function along with its arity. Naming collisions can (and do) occur, and must be handled conservatively during subsequent passes.

To avoid having to introspect third-party dependencies (which is expensive), `callgraph` uses an "accept list" of acceptable third-party functions which are known not to start any goroutines or store references to pointers (e.g. `fmt.Println`). Calls into any of these third-party functions are not included in the approximate callgraph.

Unfortunately, the decision not to introspect third-party dependencies means that any third-party function not included in the accept list *must* be treated conservatively. Thus, the `nogofunc` and `pointerescapes` passes are each triggered when a pointer is passed to such functions.

### `packid`

The `packid` analyzer simply extracts information used for package name resolution. It is used in conjunction with the accept list to avoid false-positives whenever code calls into a third-party package.

### `nogofunc`

`nogofunc` checks the declaration of each function in the codebase and marks if it starts a goroutine. It then inductively carries this information through the approximate callgraph to determine which functions do not start any goroutines.

### `pointerescapes`

The `pointerescapes` analyzer uses the approximate callgraph to find pointer arguments which may be written to memory. It checks the declaration of each function and captures which of its pointer arguments are used in an unsafe way. A pointer argument is marked as 'unsafe' if it is found either 
a) alone on the left-hand side of an assignment, or
b) within a composite literal (i.e. `Foo{1,true,&x}`)

Information for each argument is tracked separately. That is, the function `foo(x *int, y *string)` can be have `x` marked as unsafe, and `y` marked as safe.

Since the approximate callgraph allows for naming collisions, VetBot has to track these function arguments carefully. As an example, we can have functions in packages A and B with signatures `A.foo(x *int, y bar)` and `B.foo(x *string, y *int)`. In the approximate call graph, all type and package information is ignored, and both functions are represented as `{foo 2}`: the function named 'foo' with arity 2. Therefore, if the function in package B uses `x` in an unsafe way, the conservative choice is to mark the first argument of `{foo 2}` as unsafe, and treat it as such *even in invocations of `B.foo`*. We accept false positives in case a pointer is passed as the first argument of `A.foo` in order to prevent false-negatives in case a pointer is passed as the first argument to `B.foo`. This is a trade-off made to avoid dependence on the type-checker (which is expensive).

