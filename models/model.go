package models

import (
	"time"
)

type AssignmentDate struct {
	StartDate time.Time
	DueDate   time.Time
	EndDate   time.Time
}
