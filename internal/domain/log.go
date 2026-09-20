package domain

type AccessLogEntry struct {
	Time        string `json:"time"`
	FromIP      string `json:"from_ip"`
	Protocol    string `json:"protocol"`
	Target      string `json:"target"`
	Route       string `json:"route"`
	InboundTag  string `json:"inbound_tag"`
	OutboundTag string `json:"outbound_tag"`
	Email       string `json:"email"`
	Action      string `json:"action"`
	Raw         string `json:"raw"`
}

type ErrorLogEntry struct {
	Time     string `json:"time"`
	Level    string `json:"level"`
	Module   string `json:"module"`
	Message  string `json:"message"`
	SmartTip string `json:"smartTip"`
	Raw      string `json:"raw"`
}

type LogFilter struct {
	InboundTag string
	Keyword    string
}
