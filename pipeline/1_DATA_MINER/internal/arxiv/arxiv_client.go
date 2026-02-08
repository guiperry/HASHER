package arxiv

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ArxivEntry represents a single arXiv entry in the Atom feed
type ArxivEntry struct {
	XMLName         xml.Name        `xml:"entry"`
	ID              string          `xml:"id"`
	Updated         string          `xml:"updated"`
	Published       string          `xml:"published"`
	Title           string          `xml:"title"`
	Summary         string          `xml:"summary"`
	Authors         []Author        `xml:"author"`
	Links           []Link          `xml:"link"`
	Categories      []Category      `xml:"category"`
	PrimaryCategory PrimaryCategory `xml:"arxiv:primary_category"`
	Comment         string          `xml:"arxiv:comment"`
	JournalRef      string          `xml:"arxiv:journal_ref"`
	DOI             string          `xml:"arxiv:doi"`
}

// Author represents an author entry
type Author struct {
	Name        string `xml:"name"`
	Affiliation string `xml:"arxiv:affiliation"`
}

// Link represents a link entry
type Link struct {
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr"`
	Rel   string `xml:"rel,attr"`
	Title string `xml:"title,attr"`
}

// Category represents a category entry
type Category struct {
	Term   string `xml:"term,attr"`
	Scheme string `xml:"scheme,attr"`
}

// PrimaryCategory represents the primary category
type PrimaryCategory struct {
	Term   string `xml:"term,attr"`
	Scheme string `xml:"scheme,attr"`
}

// ArxivFeed represents the Atom feed from arXiv API
type ArxivFeed struct {
	XMLName      xml.Name     `xml:"feed"`
	Title        string       `xml:"title"`
	ID           string       `xml:"id"`
	Updated      string       `xml:"updated"`
	Entries      []ArxivEntry `xml:"entry"`
	TotalResults string       `xml:"opensearch:totalResults"`
	StartIndex   string       `xml:"opensearch:startIndex"`
	ItemsPerPage string       `xml:"opensearch:itemsPerPage"`
}

// ArxivClient provides functionality to interact with arXiv API
type ArxivClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewArxivClient creates a new arXiv API client
func NewArxivClient() *ArxivClient {
	return &ArxivClient{
		BaseURL: "http://export.arxiv.org/api/query",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SearchRequest represents a search request to arXiv API
type SearchRequest struct {
	Query      string   // search_query parameter
	IDList     []string // id_list parameter (optional)
	Start      int      // start parameter (default 0)
	MaxResults int      // max_results parameter (default 10)
	SortBy     string   // sortBy parameter (optional)
	SortOrder  string   // sortOrder parameter (optional)
}

// SearchByCategory searches for papers in a specific category
func (c *ArxivClient) SearchByCategory(category string, maxResults int) (*ArxivFeed, error) {
	req := SearchRequest{
		Query:      fmt.Sprintf("cat:%s", category),
		MaxResults: maxResults,
		Start:      0,
	}
	return c.Search(req)
}

// Search performs a search against the arXiv API
func (c *ArxivClient) Search(req SearchRequest) (*ArxivFeed, error) {
	// Build query parameters
	params := url.Values{}

	if req.Query != "" {
		params.Add("search_query", req.Query)
	}

	if len(req.IDList) > 0 {
		params.Add("id_list", strings.Join(req.IDList, ","))
	}

	params.Add("start", strconv.Itoa(req.Start))
	params.Add("max_results", strconv.Itoa(req.MaxResults))

	if req.SortBy != "" {
		params.Add("sortBy", req.SortBy)
	}

	if req.SortOrder != "" {
		params.Add("sortOrder", req.SortOrder)
	}

	// Build full URL
	fullURL := fmt.Sprintf("%s?%s", c.BaseURL, params.Encode())

	// Make HTTP request
	resp, err := c.HTTPClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to arXiv API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arXiv API returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse XML response
	var feed ArxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	return &feed, nil
}

// GetPDFURL extracts the PDF URL from an arXiv entry
func (e *ArxivEntry) GetPDFURL() string {
	for _, link := range e.Links {
		if link.Title == "pdf" || link.Type == "application/pdf" {
			return link.Href
		}
	}
	return ""
}

// GetArxivID extracts the arXiv ID from the entry
func (e *ArxivEntry) GetArxivID() string {
	// ID format: http://arxiv.org/abs/cs.LG/2301.07041 or http://arxiv.org/abs/2301.07041
	if strings.Contains(e.ID, "/abs/") {
		parts := strings.Split(e.ID, "/abs/")
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return ""
}

// GetPrimaryCategory returns the primary category of the paper
func (e *ArxivEntry) GetPrimaryCategory() string {
	if e.PrimaryCategory.Term != "" {
		return e.PrimaryCategory.Term
	}

	// Fallback to first category if primary is not set
	if len(e.Categories) > 0 {
		return e.Categories[0].Term
	}

	return ""
}

// CategoryInfo holds information about arXiv categories
type CategoryInfo struct {
	ID          string
	Name        string
	Description string
	Archive     string
	Group       string
}

// GetCategoryMapping returns a map of category ID to category info
func GetCategoryMapping() map[string]CategoryInfo {
	// This is a simplified subset of the full arXiv taxonomy
	// In production, this could be loaded from a configuration file or scraped
	categories := map[string]CategoryInfo{
		// Computer Science
		"cs.AI": {"cs.AI", "Artificial Intelligence", "Covers all areas of AI except Vision, Robotics, Machine Learning, Multiagent Systems, and Computation and Language", "Computer Science", "Computer Science"},
		"cs.CL": {"cs.CL", "Computation and Language", "Covers natural language processing", "Computer Science", "Computer Science"},
		"cs.CV": {"cs.CV", "Computer Vision and Pattern Recognition", "Covers image processing, computer vision, pattern recognition", "Computer Science", "Computer Science"},
		"cs.LG": {"cs.LG", "Machine Learning", "Papers on all aspects of machine learning research", "Computer Science", "Computer Science"},
		"cs.NE": {"cs.NE", "Neural and Evolutionary Computing", "Covers neural networks, connectionism, genetic algorithms", "Computer Science", "Computer Science"},
		"cs.RO": {"cs.RO", "Robotics", "Covers robotics research", "Computer Science", "Computer Science"},
		"cs.CR": {"cs.CR", "Cryptography and Security", "Covers all areas of cryptography and security", "Computer Science", "Computer Science"},
		"cs.DB": {"cs.DB", "Databases", "Covers database management, datamining, and data processing", "Computer Science", "Computer Science"},
		"cs.DS": {"cs.DS", "Data Structures and Algorithms", "Covers data structures and analysis of algorithms", "Computer Science", "Computer Science"},
		"cs.IR": {"cs.IR", "Information Retrieval", "Covers indexing, dictionaries, retrieval, content and analysis", "Computer Science", "Computer Science"},

		// Mathematics
		"math.ST": {"math.ST", "Statistics Theory", "Applied, computational and theoretical statistics", "Mathematics", "Mathematics"},
		"math.PR": {"math.PR", "Probability", "Theory and applications of probability and stochastic processes", "Mathematics", "Mathematics"},
		"math.NA": {"math.NA", "Numerical Analysis", "Numerical algorithms for problems in analysis and algebra", "Mathematics", "Mathematics"},
		"math.OC": {"math.OC", "Optimization and Control", "Operations research, linear programming, control theory", "Mathematics", "Mathematics"},

		// Physics
		"physics.data-an": {"physics.data-an", "Data Analysis, Statistics and Probability", "Methods for physics data analysis", "Physics", "Physics"},
		"physics.comp-ph": {"physics.comp-ph", "Computational Physics", "All aspects of computational science applied to physics", "Physics", "Physics"},
		"quant-ph":        {"quant-ph", "Quantum Physics", "Quantum mechanics and quantum information theory", "Physics", "Physics"},

		// Quantitative Biology
		"q-bio.BM": {"q-bio.BM", "Biomolecules", "DNA, RNA, proteins, lipids, etc.", "Quantitative Biology", "Quantitative Biology"},
		"q-bio.GN": {"q-bio.GN", "Genomics", "DNA sequencing and assembly; gene and motif finding", "Quantitative Biology", "Quantitative Biology"},
		"q-bio.NC": {"q-bio.NC", "Neurons and Cognition", "Synapse, cortex, neuronal dynamics, neural network", "Quantitative Biology", "Quantitative Biology"},
		"q-bio.QM": {"q-bio.QM", "Quantitative Methods", "Experimental, numerical and mathematical contributions to biology", "Quantitative Biology", "Quantitative Biology"},

		// Quantitative Finance
		"q-fin.CP": {"q-fin.CP", "Computational Finance", "Computational methods for financial modeling", "Quantitative Finance", "Quantitative Finance"},
		"q-fin.ST": {"q-fin.ST", "Statistical Finance", "Statistical analyses of financial markets", "Quantitative Finance", "Quantitative Finance"},

		// Statistics
		"stat.ML": {"stat.ML", "Machine Learning", "Machine learning papers with statistical grounding", "Statistics", "Statistics"},
		"stat.AP": {"stat.AP", "Applications", "Applications of statistics to various fields", "Statistics", "Statistics"},
		"stat.ME": {"stat.ME", "Methodology", "Statistical methodology and theory", "Statistics", "Statistics"},
	}

	return categories
}

// ListCategoriesForGroup returns categories for a specific group
func ListCategoriesForGroup(groupName string) []CategoryInfo {
	var categories []CategoryInfo
	allCategories := GetCategoryMapping()

	for _, category := range allCategories {
		if category.Group == groupName {
			categories = append(categories, category)
		}
	}

	return categories
}

// GetAllCategories returns all available categories
func GetAllCategories() []CategoryInfo {
	var categories []CategoryInfo
	allCategories := GetCategoryMapping()

	for _, category := range allCategories {
		categories = append(categories, category)
	}

	return categories
}
