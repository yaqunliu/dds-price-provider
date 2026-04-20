package types

type ModelPrice struct {
	ModelName       string  `json:"model_name"`
	GroupName       string  `json:"group_name"`
	InputPrice      float64 `json:"input_price"`
	OutputPrice     float64 `json:"output_price"`
	CacheInputPrice float64 `json:"cache_input_price"`
	Enabled         bool    `json:"enabled"`
	Note            string  `json:"note"`
}

type PricingData struct {
	Currency   string       `json:"currency"`
	PriceUnit  string       `json:"price_unit"`
	SiteName   string       `json:"site_name"`
	SiteDomain string       `json:"site_domain"`
	UpdatedAt  string       `json:"updated_at"`
	Models     []ModelPrice `json:"models"`
}

type PricingResponse struct {
	SchemaVersion string       `json:"schema_version"`
	Success       bool         `json:"success"`
	Message       string       `json:"message"`
	Data          *PricingData `json:"data"`
}
