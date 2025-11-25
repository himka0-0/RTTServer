package model

import (
	"RTTServer/internal/client"
	"time"
)

type RTTRecord struct {
	IP               string             `json:"ip"`
	TCPI_RTT_us      uint32             `json:"tcpi_rtt_us"`
	RTT_ms           float64            `json:"tcpi_rtt_ms"`
	TCPI_VAR_us      uint32             `json:"tcpi_rttvar_us"`
	RTTVar_ms        float64            `json:"tcpi_rttvar_ms"`
	IDProbeGlabal    string             `json:"id_probe_globalping,omitempty"`
	GlobalpingRTT    float64            `json:"globalping_rtt_ms,omitempty"`
	InfoProbes       []client.ProbeInfo `json:"info_probes,omitempty"`
	UpdatedAt        time.Time          `json:"updated_at"`
	DistanceToServer float64            `json:"distance_to_server_km,omitempty"`
}
