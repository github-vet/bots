# TrackBot

TrackBot is responsible for automating the crowd-sourcing workflow. 

## Overview

Once an issue is found, crowd-sourcing is used to classify it into one of several categories. This classification is handled via a combination of expert opinion and community effort. Gophers can provide their assessment of each issue's classification by adding emoji reactions to the issues.

TrackBot periodically scans every issue in the repository, checking the issue reactions and the issue labels. In the course of a scan, it does a few things.

1. Assess Expert Opinion
1. Assess Community Opinion
1. Detect and Alert on Expert Disagreement
1. Close Issues

### 1. Assess Expert Opinion

TrackBot maintains a list of the GitHub usernames of experts who are trusted to render a careful opinion. When TrackBot finds that an expert has reacted to an issue, a few things happen.

1. A label is added to the issue to indicate the expert opinion.
1. The reliability of community members is updated based on whether or not they agree with the expert assessment.
1. If 2 or more experts agree, the issue is closed.

### 2. Assess Community Opinion

TrackBot uses the reactions left by non-experts to update a community opinion. TrackBot keeps track of how frequently each community member has made an assessment that concurs with the assessment left by an expert. It uses this information to compute an overall reliability score, and sums this score over all users who have reacted to the issue. If the community score is high enough, a few things happen.

1. If a baseline reliability threshold is met and more than 80% of the community votes (weighted by reliability) are assigned to a single classification, a label is added to the issue to indicate the community opinion.
1. If no single classification receives more than 80% of the total community votes (weighted by reliability), a label is added to indicate the issue is confusing and may warrant higher scrutiny by an expert.
1. If the community reliability score exceeds a high reliability threshold, the issue is labeled as 'reliable'.

### 3. Assess and Alert on Expert Disagreement

TrackBot notices when experts leave conflicting opinions and takes action by leaving a comment to alert them to the issue using an `@` mention. Experts are expected to discuss and resolve any disagreement. TrackBot also labels the issue to indicate expert confusion in this case.

### 4. Close Issues

TrackBot also closes issues it finds which have the `test` or `vendored` label, and scans the file path to apply these labels if they are not already present.
