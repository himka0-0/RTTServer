package utils

import "math"

func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	LatServ, LonServ := deg2rad(lat1), deg2rad(lon1)
	LatMak, LonMak := deg2rad(lat2), deg2rad(lon2)

	dLat := LatMak - LatServ
	dLon := LonMak - LonServ

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(LatServ)*math.Cos(LatMak)*math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }
