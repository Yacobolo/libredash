package semantic_test

import (
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
)

type Model = semanticmodel.Model
type Source = semanticmodel.Source
type SourceField = semanticmodel.SourceField
type Connection = semanticmodel.Connection
type ConnectionAuth = semanticmodel.ConnectionAuth
type ConnectionDefaults = semanticmodel.ConnectionDefaults
type ModelTable = semanticmodel.Table
type ModelTransform = semanticmodel.Transform
type MetricDimension = semanticmodel.MetricDimension
type MetricMeasure = semanticmodel.MetricMeasure
type Relationship = semanticmodel.Relationship

type Dashboard = report.Dashboard
type FieldRef = report.FieldRef
type FilterDefinition = report.FilterDefinition
type FilterPreset = report.FilterPreset
type FilterTargets = report.FilterTargets
type Interaction = report.Interaction
type SelectionInteraction = report.SelectionInteraction
type SelectionMapping = report.SelectionMapping
type Visual = report.Visual
type VisualQuery = report.VisualQuery
type TableVisual = report.TableVisual
type TableQuery = report.TableQuery
