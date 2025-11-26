package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Client manages HTTP requests, session cookies, and URL mappings
type Client struct {
	httpClient  *http.Client
	loginURL    string
	baseURL     string // This will be updated after login
	urlMappings map[string]string
	orgCode     string
}

// SearchPayload defines the structure for the search request body
type SearchPayload struct {
	EhrBaseFilterMap       map[string]interface{} `json:"ehrBaseFilterMap"`
	EhrClassifyCdFilterMap map[string]interface{} `json:"ehrClassifyCdFilterMap"`
	CdRelation             string                 `json:"cdRelation"`
}

// SearchResponse defines the structure of the search results
type SearchResponse struct {
	Content []Patient `json:"content"`
}

// Patient defines the structure for a single patient record in the search response
type Patient struct {
	PersonName string `json:"personName"`
	IdNumber   string `json:"idNumber"`
	BirthDate  string `json:"birthDate"`
	Gender     string `json:"gender"`
	Death      string `json:"death"`
}

// NewClient creates a new client with a cookie jar, URL mappings, and a configurable timeout.
func NewClient(creds Credentials, mappings map[string]string) *Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Failed to create cookie jar: %s", err)
	}

	timeout := time.Duration(creds.TimeoutSeconds) * time.Second
	if creds.TimeoutSeconds <= 0 {
		timeout = 30 * time.Second // Default timeout
	}

	return &Client{
		httpClient:  &http.Client{Jar: jar, Timeout: timeout},
		loginURL:    creds.URL,
		urlMappings: mappings,
	}
}

// applyURLMappings checks if the given URL matches a mapping and returns the mapped URL.
func (c *Client) applyURLMappings(originalURL string) string {
	for internal, public := range c.urlMappings {
		if strings.Contains(originalURL, internal) {
			mappedURL := strings.Replace(originalURL, internal, public, 1)
			log.Printf("URL mapped: %s -> %s", originalURL, mappedURL)
			return mappedURL
		}
	}
	return originalURL // Return original if no mapping found
}

// DetectLoginProfile fetches the login page and determines which profile to use.
func (c *Client) DetectLoginProfile(config *ProfilesConfig) (*LoginProfile, string, error) {
	mappedLoginURL := c.applyURLMappings(c.loginURL)
	parsedURL, err := url.Parse(mappedLoginURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid mapped login URL: %w", err)
	}
	c.baseURL = fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	resp, err := c.httpClient.Get(mappedLoginURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch login page: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read login page body: %w", err)
	}
	bodyString := string(bodyBytes)

	for name, profile := range config.LoginProfiles {
		if strings.Contains(bodyString, profile.Identifier) {
			log.Printf("Detected login profile: %s", name)
			return &profile, name, nil
		}
	}

	return nil, "", fmt.Errorf("could not detect a known login profile")
}

// GetCaptcha fetches the captcha image bytes.
func (c *Client) GetCaptcha() ([]byte, error) {
	captchaURL := c.baseURL + "/phis/app/login/voCode"
	resp, err := c.httpClient.Get(captchaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch captcha image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get captcha, status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// Login dispatches to the correct login method based on the profile.
func (c *Client) Login(username, password, captcha, profileName string) error {
	hasher := md5.New()
	hasher.Write([]byte(password))
	hashedPassword := hex.EncodeToString(hasher.Sum(nil))

	switch profileName {
	case "standard":
		return c.loginType2(username, hashedPassword, captcha)
	case "alternate":
		return c.loginType1(username, hashedPassword)
	default:
		return fmt.Errorf("unknown login profile: %s", profileName)
	}
}

// loginType1 handles the complex, multi-step, no-captcha login flow.
func (c *Client) loginType1(username, hashedPassword string) error {
	// Step 1: Initial POST login
	log.Println("Login Type 1, Step 1: Initial POST")
	formData := url.Values{}
	formData.Set("j_username", username)
	formData.Set("j_password", hashedPassword)

	loginURL := c.baseURL + "/cis/j_spring_security_check"
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("step 1 failed: could not create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("step 1 failed: login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return fmt.Errorf("step 1 failed: bad status code %d", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Check if we are on the department selection page or were redirected past it.
	if strings.Contains(bodyString, "gotoDept") {
		// Scenario A: Multi-department user, we need to select one.
		log.Println("Login Type 1, Step 2: Multi-department user, parsing department page")
		re := regexp.MustCompile(`gotoDept\('([^']*)','([^']*)','([^']*)','([^']*)'\)`) // Find parameters inside gotoDept call
		matches := re.FindStringSubmatch(bodyString)
		if len(matches) < 5 {
			return fmt.Errorf("step 2 failed: could not find department info in login response")
		}
		deptID, deptCode, courtyardCode, courtyardName := matches[1], matches[2], matches[3], matches[4]
		deptURL := fmt.Sprintf("%s/cis/desktop.jsp?deptId=%s&deptCode=%s&courtyardCode=%s&courtyardName=%s",
			c.baseURL, deptID, deptCode, courtyardCode, url.QueryEscape(courtyardName))

		// Step 3: Access the department URL, which redirects to the main page
		log.Println("Login Type 1, Step 3: Accessing department URL")
		resp, err = c.httpClient.Get(deptURL)
		if err != nil {
			return fmt.Errorf("step 3 failed: could not access department page: %w", err)
		}
		defer resp.Body.Close()

		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("step 3 failed: could not read department page body: %w", err)
		}
		bodyString = string(bodyBytes) // Overwrite bodyString with the final page content
	} else {
		// Scenario B: Single-department user, we already have the final page content.
		log.Println("Login Type 1, Step 2/3: Single-department user, skipping department selection.")
	}

	// From here, the logic is the same for both scenarios, as `bodyString`
	// now contains the final page with the JS variables.

	// Step 4: Parse the final page to get JS variables
	log.Println("Login Type 1, Step 4: Parsing JS variables from department page")
	phisRe := regexp.MustCompile(`Http\.phis = \'(.+?)\'`)
	loginRe := regexp.MustCompile(`var Login=({.*?});`)
	authRe := regexp.MustCompile(`Http\.authPhis='(.*?)';`)

	phisMatches := phisRe.FindStringSubmatch(bodyString)
	if len(phisMatches) < 2 {
		return fmt.Errorf("step 4 failed: could not find Http.phis URL")
	}
	phisURL := phisMatches[1]

	authMatch := authRe.FindStringSubmatch(bodyString) // Search the whole body string
	if len(authMatch) < 2 {
		return fmt.Errorf("step 4 failed: could not find Http.authPhis token")
	}
	authToken := authMatch[1] // Use the correct index

	loginMatch := loginRe.FindStringSubmatch(bodyString)
	if len(loginMatch) < 2 {
		return fmt.Errorf("step 4 failed: could not find Login object in JS")
	}
	loginJSON := loginMatch[1]

	nameRe := regexp.MustCompile(`"fullName":"(.*?)"`)
	orgRe := regexp.MustCompile(`"domainId":"(.*?)"`)
	roleRe := regexp.MustCompile(`"accountType":"(.*?)"`)

	nameMatch := nameRe.FindStringSubmatch(loginJSON)
	orgMatch := orgRe.FindStringSubmatch(loginJSON)
	roleMatch := roleRe.FindStringSubmatch(loginJSON)

	if len(nameMatch) < 2 || len(orgMatch) < 2 || len(roleMatch) < 2 {
		return fmt.Errorf("step 4 failed: could not parse all required fields from Login object")
	}
	userName, orgCode, userRoleType := nameMatch[1], orgMatch[1], roleMatch[1]

	// Step 5: Construct the special entry URL
	log.Println("Login Type 1, Step 5: Constructing PHIS entry URL")
	internalEntryURL := fmt.Sprintf("%s/app/api/chronic/index?auth=%s&userName=%s&orgCode=%s&userRoleType=%s",
		phisURL, authToken, url.QueryEscape(userName), orgCode, userRoleType)
	c.orgCode = orgCode

	mappedEntryURL := c.applyURLMappings(internalEntryURL)

	// Step 6: Make a GET request to the entry URL to establish the PHIS session
	log.Println("Login Type 1, Step 6: Accessing PHIS entry URL")
	resp, err = c.httpClient.Get(mappedEntryURL)
	if err != nil {
		return fmt.Errorf("step 6 failed: could not access PHIS entry URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("step 6 failed: bad status code %d on PHIS entry", resp.StatusCode)
	}

	// Step 7: Update the client's base URL to the new, mapped PHIS URL for subsequent requests
	finalPhisURL := c.applyURLMappings(phisURL)
	parsedPhisURL, err := url.Parse(finalPhisURL)
	if err != nil {
		return fmt.Errorf("step 7 failed: could not parse final PHIS URL: %w", err)
	}
	c.baseURL = fmt.Sprintf("%s://%s", parsedPhisURL.Scheme, parsedPhisURL.Host)
	log.Printf("PHIS session established. Final base URL: %s", c.baseURL)

	return nil
}

// loginType2 handles the captcha-based login flow.
func (c *Client) loginType2(username, hashedPassword, captcha string) error {
	formData := url.Values{}
	formData.Set("phisname", username)
	formData.Set("password", hashedPassword)
	formData.Set("verifyCode", captcha)
	formData.Set("orgCode", "06") // This is a default, but we will extract the real one from the response
	formData.Set("navigation", "isNotNavigation")

	loginURL := c.baseURL + "/phis/app/login"
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	// Read the response body to find the user object and extract the orgCode
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body after login: %w", err)
	}
	bodyString := string(bodyBytes)

	// Extract orgCode from `var user = { ... }`
	userRe := regexp.MustCompile(`var user = ({[\s\S]*?});`)
	userMatch := userRe.FindStringSubmatch(bodyString)
	if len(userMatch) < 2 {
		if strings.Contains(bodyString, "loginFailure") {
			return fmt.Errorf("login failed, please check credentials and captcha")
		}
		return fmt.Errorf("could not find user object in response page, login may have failed")
	}
	userBlock := userMatch[1]

	orgRe := regexp.MustCompile(`orgCode\s*:\s*'(.*?)'`)
	orgMatch := orgRe.FindStringSubmatch(userBlock)
	if len(orgMatch) < 2 {
		return fmt.Errorf("could not find orgCode in user object")
	}

	c.orgCode = orgMatch[1]
	log.Printf("Login successful (Type 2). orgCode extracted: %s", c.orgCode)

	return nil
}

// SearchDeadPatients performs the search request for deceased patients
func (c *Client) SearchDeadPatients() ([]Patient, error) {
	payload := SearchPayload{
		EhrBaseFilterMap: map[string]interface{}{
			"EQ_death":       1,
			"LIKE_innerCode": "%",
			"LIKE_ehrCode":   "%",
			"EQ_mngOrgCode":  c.orgCode,
			"LIKE_nameIndex": "%",
		},
		EhrClassifyCdFilterMap: map[string]interface{}{
			"EQ_cdHypertension":     1,
			"EQ_cdDiabetesMellitus": 1,
		},
		CdRelation: "OR",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search payload: %w", err)
	}

	searchURL := c.baseURL + "/phis/app/ehr?sort=dateCreated&dir=ASC&limit=100&start=0"
	req, err := http.NewRequest("POST", searchURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status %d", resp.StatusCode)
	}

	var searchResult SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	log.Printf("Found %d deceased patients.", len(searchResult.Content))
	return searchResult.Content, nil
}
