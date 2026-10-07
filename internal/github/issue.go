package github

import (
	"context"
	"strconv"
)

// nodeQuery resolves either an Issue or a PullRequest — they share numbering,
// so /issues/N still works when N is really a PR. Both carry status transitions.
const nodeQuery = `
query($owner:String!, $repo:String!, $number:Int!) {
  repository(owner:$owner, name:$repo) {
    issueOrPullRequest(number:$number) {
      __typename
      ... on Issue {
        number title url
        parent { url }
        subIssues(first:50) { nodes { url } }
        projectItems(first:10) { nodes { fieldValues(first:30) { nodes {
          ... on ProjectV2ItemFieldDateValue { date field { ... on ProjectV2FieldCommon { name } } }
        } } } }
        timelineItems(first:100, itemTypes:[
          PROJECT_V2_ITEM_STATUS_CHANGED_EVENT, CONNECTED_EVENT, CROSS_REFERENCED_EVENT, CLOSED_EVENT
        ]) { nodes {
          __typename
          ... on ProjectV2ItemStatusChangedEvent { createdAt status }
          ... on ClosedEvent { createdAt }
          ... on ConnectedEvent { createdAt subject { ... on Issue { url } ... on PullRequest { url } } }
          ... on CrossReferencedEvent { createdAt source { ... on Issue { url } ... on PullRequest { url } } }
        } }
      }
      ... on PullRequest {
        number title url mergedAt
        projectItems(first:10) { nodes { fieldValues(first:30) { nodes {
          ... on ProjectV2ItemFieldDateValue { date field { ... on ProjectV2FieldCommon { name } } }
        } } } }
        timelineItems(first:100, itemTypes:[
          PROJECT_V2_ITEM_STATUS_CHANGED_EVENT, CONNECTED_EVENT, CROSS_REFERENCED_EVENT, CLOSED_EVENT, MERGED_EVENT
        ]) { nodes {
          __typename
          ... on ProjectV2ItemStatusChangedEvent { createdAt status }
          ... on ClosedEvent { createdAt }
          ... on MergedEvent { createdAt }
          ... on ConnectedEvent { createdAt subject { ... on Issue { url } ... on PullRequest { url } } }
          ... on CrossReferencedEvent { createdAt source { ... on Issue { url } ... on PullRequest { url } } }
        } }
      }
    }
  }
}`

type Ref struct {
	Owner  string
	Repo   string
	Number int
}

func (r Ref) URL() string {
	return "https://github.com/" + r.Owner + "/" + r.Repo + "/issues/" + strconv.Itoa(r.Number)
}

type linkRef struct {
	URL string `json:"url"`
}

type timelineNode struct {
	Type      string  `json:"__typename"`
	CreatedAt string  `json:"createdAt"`
	Status    string  `json:"status"`
	Source    linkRef `json:"source"`
	Subject   linkRef `json:"subject"`
}

type node struct {
	Type      string  `json:"__typename"`
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	MergedAt  string  `json:"mergedAt"`
	Parent    linkRef `json:"parent"`
	SubIssues struct {
		Nodes []linkRef `json:"nodes"`
	} `json:"subIssues"`
	ProjectItems struct {
		Nodes []struct {
			FieldValues struct {
				Nodes []struct {
					Date  string `json:"date"`
					Field struct {
						Name string `json:"name"`
					} `json:"field"`
				} `json:"nodes"`
			} `json:"fieldValues"`
		} `json:"nodes"`
	} `json:"projectItems"`
	TimelineItems struct {
		Nodes []timelineNode `json:"nodes"`
	} `json:"timelineItems"`
}

func (c *Client) FetchNode(ctx context.Context, ref Ref) (*Scan, error) {
	var data struct {
		Repository struct {
			Node node `json:"issueOrPullRequest"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": ref.Owner, "repo": ref.Repo, "number": ref.Number}
	if err := c.query(ctx, nodeQuery, vars, &data); err != nil {
		return nil, err
	}
	return nodeToScan(data.Repository.Node), nil
}

func nodeToScan(n node) *Scan {
	scan := &Scan{URL: n.URL, Title: n.Title}

	for _, item := range n.ProjectItems.Nodes {
		for _, fv := range item.FieldValues.Nodes {
			if fv.Date != "" && fieldIsTargetDate(fv.Field.Name) {
				scan.TargetDate = fv.Date
			}
		}
	}

	linked := map[string]struct{}{}
	addLink := func(u string) {
		if nu := NormalizeURL(u); nu != "" && nu != n.URL {
			linked[nu] = struct{}{}
		}
	}
	addLink(n.Parent.URL)
	for _, s := range n.SubIssues.Nodes {
		addLink(s.URL)
	}

	for _, ev := range n.TimelineItems.Nodes {
		t, ok := parseTime(ev.CreatedAt)
		if !ok {
			continue
		}
		switch ev.Type {
		case "ProjectV2ItemStatusChangedEvent":
			scan.Events = append(scan.Events, Event{DateTime: t, TargetStatus: NormalizeStatus(ev.Status)})
		case "ClosedEvent":
			scan.Events = append(scan.Events, Event{DateTime: t, ClosedThis: true, WorkActivity: true})
		case "MergedEvent":
			scan.Events = append(scan.Events, Event{DateTime: t, WorkActivity: true})
		case "ConnectedEvent":
			addLink(ev.Subject.URL)
			scan.Events = append(scan.Events, Event{DateTime: t, WorkActivity: true})
		case "CrossReferencedEvent":
			addLink(ev.Source.URL)
			scan.Events = append(scan.Events, Event{DateTime: t, WorkActivity: true})
		}
	}

	for u := range linked {
		scan.LinkedURLs = append(scan.LinkedURLs, u)
	}
	return scan
}

func fieldIsTargetDate(name string) bool {
	switch name {
	case "Target Date", "Target date", "target date":
		return true
	default:
		return false
	}
}
