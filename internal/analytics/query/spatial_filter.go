package query

import (
	"fmt"
	"math"
	"strings"
)

const (
	maximumSpatialLassoPoints  = 256
	maximumSpatialRadiusMeters = 5_000_000
	earthMeanRadiusMeters      = 6_371_008.8
)

// ValidateSpatialFilter applies the same closed, exact validation used by SQL
// planning without exposing renderer or SQL implementation details.
func ValidateSpatialFilter(filter SpatialFilter) error {
	_, _, err := spatialFilterSQL("latitude", "longitude", filter)
	return err
}

func spatialFilterSQL(latitudeExpr, longitudeExpr string, filter SpatialFilter) (string, []any, error) {
	if strings.TrimSpace(latitudeExpr) == "" || strings.TrimSpace(longitudeExpr) == "" {
		return "", nil, fmt.Errorf("spatial filter requires coordinate expressions")
	}
	switch filter.Kind {
	case "box":
		if err := validateSpatialBounds(filter.West, filter.South, filter.East, filter.North); err != nil {
			return "", nil, err
		}
		latitude := fmt.Sprintf("(%s >= ? AND %s <= ?)", latitudeExpr, latitudeExpr)
		if filter.West < filter.East {
			return fmt.Sprintf("(%s AND %s >= ? AND %s <= ?)", latitude, longitudeExpr, longitudeExpr), []any{filter.South, filter.North, filter.West, filter.East}, nil
		}
		return fmt.Sprintf("(%s AND (%s >= ? OR %s <= ?))", latitude, longitudeExpr, longitudeExpr), []any{filter.South, filter.North, filter.West, filter.East}, nil
	case "lasso":
		return spatialLassoSQL(latitudeExpr, longitudeExpr, filter.Points)
	case "radius":
		if err := validateSpatialPoint(filter.Center); err != nil {
			return "", nil, fmt.Errorf("spatial radius center: %w", err)
		}
		if !finite(filter.RadiusMeters) || filter.RadiusMeters <= 0 || filter.RadiusMeters > maximumSpatialRadiusMeters {
			return "", nil, fmt.Errorf("spatial radius must be greater than zero and at most %.0f meters", float64(maximumSpatialRadiusMeters))
		}
		sql := fmt.Sprintf("(2 * %.1f * ASIN(SQRT(POWER(SIN(RADIANS(%s - ?) / 2), 2) + COS(RADIANS(?)) * COS(RADIANS(%s)) * POWER(SIN(RADIANS(%s - ?) / 2), 2))) <= ?)", earthMeanRadiusMeters, latitudeExpr, latitudeExpr, longitudeExpr)
		return sql, []any{filter.Center.Latitude, filter.Center.Latitude, filter.Center.Longitude, filter.RadiusMeters}, nil
	default:
		return "", nil, fmt.Errorf("unsupported spatial filter kind %q", filter.Kind)
	}
}

func spatialLassoSQL(latitudeExpr, longitudeExpr string, points []SpatialPoint) (string, []any, error) {
	if len(points) < 3 || len(points) > maximumSpatialLassoPoints {
		return "", nil, fmt.Errorf("spatial lasso requires between 3 and %d points", maximumSpatialLassoPoints)
	}
	west, east := math.Inf(1), math.Inf(-1)
	south, north := math.Inf(1), math.Inf(-1)
	for _, point := range points {
		if err := validateSpatialPoint(point); err != nil {
			return "", nil, fmt.Errorf("spatial lasso: %w", err)
		}
		west, east = math.Min(west, point.Longitude), math.Max(east, point.Longitude)
		south, north = math.Min(south, point.Latitude), math.Max(north, point.Latitude)
	}
	if east-west >= 180 {
		return "", nil, fmt.Errorf("spatial lasso may not cross the antimeridian")
	}
	if south == north || west == east {
		return "", nil, fmt.Errorf("spatial lasso must enclose a non-zero area")
	}
	parts := make([]string, len(points))
	args := []any{south, north, west, east}
	for index, start := range points {
		end := points[(index+1)%len(points)]
		parts[index] = fmt.Sprintf("CASE WHEN ((? > %s) <> (? > %s)) AND %s < ((? - ?) * (%s - ?) / NULLIF(? - ?, 0) + ?) THEN 1 ELSE 0 END", latitudeExpr, latitudeExpr, longitudeExpr, latitudeExpr)
		args = append(args, end.Latitude, start.Latitude, end.Longitude, start.Longitude, start.Latitude, end.Latitude, start.Latitude, start.Longitude)
	}
	sql := fmt.Sprintf("(%s >= ? AND %s <= ? AND %s >= ? AND %s <= ? AND MOD((%s), 2) = 1)", latitudeExpr, latitudeExpr, longitudeExpr, longitudeExpr, strings.Join(parts, " + "))
	return sql, args, nil
}

func validateSpatialBounds(west, south, east, north float64) error {
	if !finite(west) || !finite(south) || !finite(east) || !finite(north) || west < -180 || west > 180 || east < -180 || east > 180 || south < -90 || south > 90 || north < -90 || north > 90 || south >= north || west == east {
		return fmt.Errorf("invalid spatial bounds")
	}
	return nil
}

func validateSpatialPoint(point SpatialPoint) error {
	if !finite(point.Longitude) || !finite(point.Latitude) || point.Longitude < -180 || point.Longitude > 180 || point.Latitude < -90 || point.Latitude > 90 {
		return fmt.Errorf("invalid spatial coordinate")
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
