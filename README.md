# bots

bots contains two bots, vetbot and trackbot.

vetbot automates the analysis of large quantities of Golang code stored in GitHub repositories. It is a special-purpose bot built to gather a large suite of examples of the well-known [range loop capture error](https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable) found "in the wild".

Range-loop capture is the reason this code prints `4, 4, 4, 4,` instead of what you might expect.

```go
xs := []int{1, 2, 3, 4}
for _, x := range xs {
    go func() {
        fmt.Printf("%d, " x)
    }()
}
```

trackbot tracks community contributions to issues raised by vetbot.

But why build bots?

## Range Loop Capture Considered Dangerous

Members of the Go language team have indicated a willingness to modify the behavior of range loop variable capture to make the behavior of Go more intuitive. This change could theoretically be made despite the [strong backwards compatibility guarantee](https://golang.org/doc/go1compat) of Go version 1, **only if** we can ensure that the change will not result in incorrect behavior in current programs.

To make that determination, a large number of "real world" `go` programs would need to be vetted. If we find that, in every case, the current compiler behavior results in an undesirable outcome (aka bugs), we can consider making a change to the language.

The goal of the [github-vet](https://github.com/github-vet) project is to motivate such a change by gathering static analysis results from Go code hosted in publicly available GitHub repositories, and crowd-sourcing their human analysis.

## How Does It Work?

vet-bot samples from a list of GitHub repositories hosting Go code, parses every `.go` file found, and runs it through [an augmented version of the loopclosure analysis](https://github.com/github-vet/vet-bot/blob/main/cmd/vet-bot/loopclosure/loopclosure.go) run as part of `go vet`. Any issues it finds are recorded to [a GitHub repository](https://github.com/github-vet/rangeloop-findings) for humans to analyze.

The static analysis procedure uses only syntactic information produced by the Go parser. It detects instances of variables in a `for` loop which escape via use in a function whose execution is delayed via `go` or `defer`. The procedure is based on [the procedure used in `go vet`](https://github.com/golang/tools/blob/master/go/analysis/passes/loopclosure/loopclosure.go), except it does not rely on type-checking information (which is hard to obtain) and works with nested loops.

## How Can I Help?

Head over to [the findings repository](https://github.com/github-vet/rangeloop-pointer-findings) to dive in and help! We are also looking for Golang experts to provide high-quality review of our findings. If you're an expert, please apply for consideration and we'll happily assign you some code to read!

## No Really, How Does It Work?

There are two bots, VetBot and TrackBot. VetBot is responsible for finding issues in Go repositories on GitHub. TrackBot is responsible for managing the community crowd-sourcing effort.

VetBot starts from a list of GitHub repositories to read from. It reads the default branch in each repository as a tarball, parsing any `.go` files it finds. Once it's built the parse tree of the entire repository, it runs two static analyzers tailored to the rangeloop capture problem. If either of these analyzers report an issue for a section of code, VetBot opens an issue on [a specific repository](https://github.com/github-vet/rangeloop-pointer-findings) which contains the segment of code that triggered the analyzer, and a link back to the repository where the code was found.

TrackBot runs periodically. Each time it wakes up, it reads through every issue in [the target repository](https://github.com/github-vet/rangeloop-pointer-findings). When it finds any issue that is not tagged properly, it updates the tags. It checks through the reactions left on every issue and uses them to update the community and expert opinions around the issue. When an expert leaves an opinion on an issue, the issue is closed. TrackBot also takes into account how often each account that has left a reaction has agreed with the expert opinion, and uses this to determine when enough reliable feedback has been given to make an assessment.

Both VetBot and TrackBot respect the rate-limits on GitHub's API.

For a more detailed overview, checkout the READMEs for TrackBot and VetBot.
