package data

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/walkccc/greenlight/internal/validator"
)

func TestValidateMovie_ValidMovie(t *testing.T) {
	movie := &Movie{
		Title:   "Test Movie",
		Year:    2023,
		Runtime: 120,
		Genres:  []string{"Action", "Thriller"},
	}

	v := validator.New()
	ValidateMovie(v, movie)

	if !v.Valid() {
		t.Errorf("expected no validation errors, got %d errors", len(v.Errors))
	}
}

func TestValidateMovie_InvalidMovie(t *testing.T) {
	movie := &Movie{
		Title:   "",         // Invalid: empty title
		Year:    1894,       // Invalid: year less than or equal to 1894
		Runtime: 0,          // Invalid: runtime not provided
		Genres:  []string{}, // Invalid: empty genres slice
	}

	v := validator.New()
	ValidateMovie(v, movie)

	if v.Valid() {
		t.Error("expected validation errors, but got none")
	}

	expectedErrors := map[string]string{
		"title":   "must be provided",
		"year":    "must be greater than 1894",
		"runtime": "must be provided",
		"genres":  "must contain at least 1 genre",
	}
	for field, expectedMessage := range expectedErrors {
		if errMsg := v.Errors[field]; errMsg != expectedMessage {
			t.Errorf(
				"expected validation error for field '%s' with message '%s', got '%s'",
				field,
				expectedMessage,
				errMsg,
			)
		}
	}
}

func NewMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	assert.Nil(t, err)
	return db, mock
}

func TestMovieModel_Get(t *testing.T) {
	createdAt, _ := time.Parse("2006-01-02", "2022-01-01")
	query := `
		SELECT id, created_at, title, year, runtime, genres, version
		FROM movies
		WHERE id = \$1
	`

	tests := []struct {
		name       string
		buildMock  func(mock sqlmock.Sqlmock)
		checkModel func(model MovieModel)
	}{
		{
			name: "Success",
			buildMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.
					NewRows(
						[]string{
							"id",
							"created_at",
							"title",
							"year",
							"runtime",
							"genres",
							"version",
						},
					).
					AddRow(1, createdAt, "Test Movie 1", 2022, 120, "{}", 1)
				mock.ExpectQuery(query).WithArgs(1).WillReturnRows(rows)
			},
			checkModel: func(model MovieModel) {
				movie, err := model.Get(1)
				assert.NotNil(t, movie)
				assert.Nil(t, err)
				assert.Equal(t, int64(1), movie.ID, "wrong id")
				assert.Equal(t, createdAt, movie.CreatedAt, "wrong created_at")
				assert.Equal(t, "Test Movie 1", movie.Title, "wrong title")
				assert.Equal(t, int32(2022), movie.Year, "wrong year")
				assert.Equal(t, int32(120), int32(movie.Runtime), "wrong runtime")
				assert.Equal(t, []string{}, movie.Genres, "wrong genres")
				assert.Equal(t, int32(1), movie.Version, "wrong version")
			},
		},
		{
			name:      "InvalidID",
			buildMock: func(mock sqlmock.Sqlmock) {},
			checkModel: func(model MovieModel) {
				movie, err := model.Get(0)
				assert.Nil(t, movie)
				assert.Equal(t, ErrRecordNotFound, err)
			},
		},
		{
			name: "ErrNoRows",
			buildMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(1).WillReturnError(sql.ErrNoRows)
			},
			checkModel: func(model MovieModel) {
				movie, err := model.Get(1)
				assert.Nil(t, movie)
				assert.Equal(t, ErrRecordNotFound, err)
			},
		},
		{
			name: "ErrConnDone",
			buildMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(1).WillReturnError(sql.ErrConnDone)
			},
			checkModel: func(model MovieModel) {
				movie, err := model.Get(1)
				assert.Nil(t, movie)
				assert.Equal(t, sql.ErrConnDone, err)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock := NewMock(t)
			model := MovieModel{DB: db}
			defer model.DB.Close()
			test.buildMock(mock)
			test.checkModel(model)
		})
	}
}

func TestMovieModel_GetAll(t *testing.T) {
	createdAt, _ := time.Parse("2006-01-02", "2022-01-01")
	filters := Filters{
		Page:           1,
		PageSize:       20,
		Sort:           "-title",
		SortSafeValues: []string{"-title"},
	}
	query := `
		SELECT
			count\(\*\) OVER\(\), id, created_at, title, year, runtime, genres, version
		FROM movies
		WHERE
			\(to_tsvector\('simple', title\) @@ plainto_tsquery\('simple', \$1\) OR \$1 = ''\)
			AND \(genres @> \$2 OR \$2 = '{}'\)
		ORDER BY title DESC, id ASC
		LIMIT \$3 OFFSET \$4
	`

	tests := []struct {
		name       string
		buildMock  func(mock sqlmock.Sqlmock)
		checkModel func(model MovieModel)
	}{
		{
			name: "SortByTitleDesc",
			buildMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.
					NewRows(
						[]string{
							"total_records",
							"id",
							"created_at",
							"title",
							"year",
							"runtime",
							"genres",
							"version",
						},
					).
					AddRow(2, 2, createdAt, "Test Funny Movie", 2022, 99, "{}", 1).
					AddRow(2, 1, createdAt, "Test Boring Movie", 2020, 99, "{}", 1)
				mock.ExpectQuery(query).
					WithArgs("Movie", pq.Array([]string{}), 20, 0).
					WillReturnRows(rows)
			},
			checkModel: func(model MovieModel) {
				movies, metadata, err := model.GetAll("Movie", []string{}, filters)
				assert.Nil(t, err)
				assert.NotNil(t, movies)
				assert.NotNil(t, metadata)
				assert.Equal(t, 2, len(movies))
				assert.Equal(t, "Test Funny Movie", movies[0].Title)
				assert.Equal(t, "Test Boring Movie", movies[1].Title)
			},
		},
		{
			name: "ErrConnDone",
			buildMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).
					WithArgs("Movie", pq.Array([]string{}), 20, 0).
					WillReturnError(sql.ErrConnDone)
			},
			checkModel: func(model MovieModel) {
				movies, metadata, err := model.GetAll("Movie", []string{}, filters)
				assert.Nil(t, movies)
				assert.Equal(t, Metadata{}, metadata)
				assert.Equal(t, sql.ErrConnDone, err)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock := NewMock(t)
			model := MovieModel{DB: db}
			defer model.DB.Close()
			test.buildMock(mock)
			test.checkModel(model)
		})
	}
}

func TestMovielModel_Update(t *testing.T) {
	createdAt, _ := time.Parse("2006-01-02", "2022-01-01")
	query := `
		UPDATE movies
		SET title = \$1,
			year = \$2,
			runtime = \$3,
			genres = \$4,
			version = version \+ 1
		WHERE id = \$5
			AND version = \$6
		RETURNING version
	`

	tests := []struct {
		name       string
		buildMock  func(mock sqlmock.Sqlmock)
		checkModel func(model MovieModel)
	}{
		{
			name: "UpdateTitle",
			buildMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"version"}).AddRow(2)
				mock.ExpectQuery(query).
					WithArgs("Updated Movie", 2022, 99, pq.Array([]string{"Sci-fi"}), 1, 1).
					WillReturnRows(rows)
			},
			checkModel: func(model MovieModel) {
				movie := &Movie{
					ID:        1,
					CreatedAt: createdAt,
					Title:     "Updated Movie",
					Year:      2022,
					Runtime:   99,
					Genres:    []string{"Sci-fi"},
					Version:   1,
				}
				err := model.Update(movie)
				assert.Nil(t, err)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock := NewMock(t)
			model := MovieModel{DB: db}
			defer model.DB.Close()
			test.buildMock(mock)
			test.checkModel(model)
		})
	}
}
