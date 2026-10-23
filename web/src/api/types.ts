import type { components } from './schema'

// The API's types the app passes around, by their names in the app.
export type Day = components['schemas']['DayBody']
export type Meal = components['schemas']['MealBody']
export type Item = components['schemas']['ItemBody']
export type Metric = components['schemas']['MetricBody']
export type Nutrients = components['schemas']['NutrientsBody']
export type Warning = components['schemas']['WarningBody']
