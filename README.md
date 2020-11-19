# vet-bot

vet-bot automates the analysis of large quantities of Golang code stored in GitHub repositories. It is a special-purpose bot built to gather a large suite of examples of the well-known [range loop capture error](https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable) found "in the wild".

Range-loop capture is the reason this code prints `4, 4, 4, 4,` instead of what you might expect.

```go
xs := []int{1, 2, 3, 4}
for _, x := range xs {
    go func() {
        fmt.Printf("%d, " x)
    }()
}
```

But why build a bot?

## Range Loop Capture Considered Dangerous

Members of the Go language team have indicated a willingness to modify the behavior of range loop variable capture to make the behavior of go more intuitive. This change could theoretically be made despite the [strong backwards compatibility guarantee](https://golang.org/doc/go1compat) of Go version 1, **only if** we can ensure that the change will not result in incorrect behavior in current programs.

To make that determination, a large number of "real world" `go` programs would need to be vetted. If we find that, in every case, the current compiler behavior results in an undesirable outcome (aka bugs), we can consider making a change to the language.

The goal of the [github-vet](https://github.com/github-vet) project is to motivate such a change by gathering static analysis results from Go code hosted in publicly available GitHub repositories, and crowd-sourcing their human analysis.

## How Does It Work?

vet-bot samples from a list of GitHub repositories hosting Go code, parses every `.go` file found, and runs it through [an augmented version of the loopclosure analysis](https://github.com/github-vet/vet-bot/blob/main/cmd/vet-bot/loopclosure/loopclosure.go) run as part of `go vet`. Any issues it finds are recorded to [a GitHub repository](https://github.com/github-vet/rangeloop-findings) for humans to analyze.

The static analysis procedure uses only syntactic information produced by the Go parser. It detects instances of variables in a `for` loop which escape via use in a function whose execution is delayed via `go` or `defer`. The procedure is based on [the procedure used in `go vet`](https://github.com/golang/tools/blob/master/go/analysis/passes/loopclosure/loopclosure.go), except it does not rely on type-checking information (which is hard to obtain) and works with nested loops.

## How Can I Help?

Head over to [the findings repository](https://github.com/github-vet/rangeloop-findings) to dive in and help! 