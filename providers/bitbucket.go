package providers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/pusher/oauth2_proxy/api"
)

type BitbucketProvider struct {
	*ProviderData
	Team  string
	Group string
}

func NewBitbucketProvider(p *ProviderData) *BitbucketProvider {
	p.ProviderName = "Bitbucket"
	if p.LoginURL == nil || p.LoginURL.String() == "" {
		p.LoginURL = &url.URL{
			Scheme: "https",
			Host:   "bitbucket.org",
			Path:   "/site/oauth2/authorize",
		}
	}
	if p.RedeemURL == nil || p.RedeemURL.String() == "" {
		p.RedeemURL = &url.URL{
			Scheme: "https",
			Host:   "bitbucket.org",
			Path:   "/site/oauth2/access_token",
		}
	}
	if p.ValidateURL == nil || p.ValidateURL.String() == "" {
		p.ValidateURL = &url.URL{
			Scheme: "https",
			Host:   "api.bitbucket.org",
			Path:   "/2.0/user/emails",
		}
	}
	if p.Scope == "" {
		p.Scope = "account team"
	}
	return &BitbucketProvider{ProviderData: p}
}

func (p *BitbucketProvider) SetTeam(team string) {
	p.Team = team
}

func (p *BitbucketProvider) SetGroup(group string) {
	p.Group = group
}

func debug(data []byte, err error) {
	if err == nil {
		fmt.Printf("%s\n\n", data)
	} else {
		log.Fatalf("%s\n\n", err)
	}
}

func (p *BitbucketProvider) GetEmailAddress(s *SessionState) (string, error) {

	var emails struct {
		Values []struct {
			Email   string `json:"email"`
			Primary bool   `json:"is_primary"`
		}
	}
	var teams struct {
		Values []struct {
			Name string `json:"username"`
		}
	}
	req, err := http.NewRequest("GET",
		p.ValidateURL.String()+"?access_token="+s.AccessToken, nil)
	if err != nil {
		log.Printf("failed building request %s", err)
		return "", err
	}
	err = api.RequestJSON(req, &emails)
	if err != nil {
		log.Printf("failed making request %s", err)
		debug(httputil.DumpRequestOut(req, true))
		return "", err
	}

	if p.Team != "" {
		log.Printf("Filtering against membership in team %s\n", p.Team)
		teamURL := &url.URL{}
		*teamURL = *p.ValidateURL
		teamURL.Path = "/2.0/teams"
		req, err = http.NewRequest("GET",
			teamURL.String()+"?role=member&access_token="+s.AccessToken, nil)
		if err != nil {
			log.Printf("failed building request %s", err)
			return "", err
		}
		err = api.RequestJSON(req, &teams)
		if err != nil {
			log.Printf("failed requesting teams membership %s", err)
			debug(httputil.DumpRequestOut(req, true))
			return "", err
		}
		var found = false
		log.Printf("%+v\n", teams)
		for _, team := range teams.Values {
			if p.Team == team.Name {
				found = true
				break
			}
		}
		if found != true {
			log.Printf("team membership test failed, access denied")
			return "", nil
		}

		if p.Group != "" {
			err = p.CheckGroupMembership(s)
			if err != nil {
				return "", err
			}
		}
	}

	for _, email := range emails.Values {
		if email.Primary {
			return email.Email, nil
		}
	}

	return "", nil
}

func (p *BitbucketProvider) CheckGroupMembership(s *SessionState) error {
	log.Printf("Checking if user belongs to group %s", p.Group)

	var user struct {
		Username  string `json:"username"`
		AccountID string `json:"account_id"`
	}
	var groupMembers []struct {
		AccountID string `json:"account_id"`
	}

	// Get the username and account_id
	userURL := &url.URL{}
	*userURL = *p.ValidateURL
	userURL.Path = "/2.0/user"
	req, err := http.NewRequest("GET",
		userURL.String()+"?role=member&access_token="+s.AccessToken, nil)
	if err != nil {
		log.Printf("failed building request %s", err)
		return err
	}

	err = api.RequestJSON(req, &user)
	if err != nil {
		log.Printf("failed requesting user details %s", err)
		debug(httputil.DumpRequestOut(req, true))
		return err
	}

	log.Printf("Bitbucket authed username=%s account_id=%s", user.Username, user.AccountID)

	if user.AccountID == "" {
		return errors.New("Could not find bitbucket account_id")
	}

	// Get members of group
	groupURL := &url.URL{}
	*groupURL = *p.ValidateURL
	groupURL.Path = fmt.Sprintf("/1.0/groups/%s/%s/members", p.Team, p.Group)
	req, err = http.NewRequest("GET",
		groupURL.String()+"?role=member&access_token="+s.AccessToken, nil)
	if err != nil {
		log.Printf("failed building request %s", err)
		return err
	}

	err = api.RequestJSON(req, &groupMembers)
	if err != nil {
		log.Printf("failed requesting group members %s", err)
		debug(httputil.DumpRequestOut(req, true))
		return err
	}

	log.Printf("Got group members %v", groupMembers)

	for _, member := range groupMembers {
		if user.AccountID == member.AccountID {
			log.Printf("Found user in group member list")
			return nil
		}
	}

	err = errors.New("User not found in group list")
	log.Print(err)

	return err
}
