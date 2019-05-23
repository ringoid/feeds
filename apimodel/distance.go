package apimodel

import (
	"math"
)

const rEarth = 6372800 //m

type pos struct {
	φ float64 // latitude, radians
	ψ float64 // longitude, radians
}

func haversine(θ float64) float64 {
	return .5 * (1 - math.Cos(θ))
}

func Point(lat, lon float64) pos {
	return pos{lat * math.Pi / 180, lon * math.Pi / 180}
}

func Distance(p1, p2 pos) float64 {
	return 2 * rEarth * math.Asin(math.Sqrt(haversine(p2.φ-p1.φ) +
		math.Cos(p1.φ)*math.Cos(p2.φ)*haversine(p2.ψ-p1.ψ)))
}
