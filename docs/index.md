# Overview

**gocrud** is a Go library designed to streamline the development of CRUDL (Create, Read, Update, Delete, List) operations for SQL-backed applications. By leveraging reflection, gocrud automatically generates a database persistence layer from Go structs containing basic field types (such as int, string, time.Time, etc.).

The library is intended to provide a database persistence layer. Maps Go structs to database tables, handling schema creation and data operations.

## Supported Databases

| Database   | Driver                     |
|------------|----------------------------|
| PostgreSQL | `github.com/lib/pq`        |
| SQLite     | `modernc.org/sqlite`       |

## Motivation

**gocrud** was created to address the need for rapidly building data persistence layers during prototyping phases for internal customer systems. The goal is to enable developers to go from a struct definition to a functional data management layer with minimal configuration—ideal for prototypes, internal tools, MVPs, or any scenario where speed and simplicity matter.

## Demo

Please navigate to [Demo](demo.md) see sample usage.