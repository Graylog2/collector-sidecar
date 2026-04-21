// Copyright (C)  2026 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.
//
// SPDX-License-Identifier: SSPL-1.0

// This program modifies a generated OTel Collector main.go source file to insert
// customization callbacks in the runInteractive function.
// This allows further customization of the collector settings and cobra command.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
)

const (
	settingsCallback = "customizeSettings"
	commandCallback  = "customizeCommand"
)

func main() {
	mainPath := flag.String("main-path", "", "path to the generated OTel Collector main.go file to modify")
	windowsMainPath := flag.String("windows-main-path", "", "path to the generated OTel Collector main_windows.go file to modify")
	flag.Parse()

	if *mainPath == "" && *windowsMainPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: at least one of -main-path or -windows-main-path is required")
		os.Exit(1)
	}

	if *mainPath != "" {
		if err := addCustomizationCalls(*mainPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if *windowsMainPath != "" {
		if err := addSupervisorDispatch(*windowsMainPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}

const supervisorDispatchCallback = "maybeSupervisorService"

// addSupervisorDispatch inserts a supervisor dispatch call at the beginning of
// the run() function in main_windows.go. This allows the supervisor's Windows
// service handler to intercept before the OTel Collector's handler runs.
func addSupervisorDispatch(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	found := false
	alreadyExists := false
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "run" {
			return true
		}

		for _, stmt := range fn.Body.List {
			ifStmt, ok := stmt.(*ast.IfStmt)
			if !ok || ifStmt.Init == nil {
				continue
			}
			assignStmt, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok {
				continue
			}
			callExpr, ok := assignStmt.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			if ident, ok := callExpr.Fun.(*ast.Ident); ok && ident.Name == supervisorDispatchCallback {
				alreadyExists = true
				return false
			}
		}

		// Remove comments inside the function body that would be misplaced
		// by the AST insertion. Go's formatter associates comments by position,
		// so prepending a statement shifts everything.
		bodyStart := fn.Body.Lbrace
		firstStmtPos := fn.Body.List[0].Pos()
		var kept []*ast.CommentGroup
		for _, cg := range f.Comments {
			if cg.Pos() > bodyStart && cg.End() < firstStmtPos {
				continue
			}
			kept = append(kept, cg)
		}
		f.Comments = kept

		// Build: if handled, err := maybeSupervisorService(params); handled { return err }
		dispatchCall := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("handled"), ast.NewIdent("err")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{
				&ast.CallExpr{
					Fun:  ast.NewIdent(supervisorDispatchCallback),
					Args: []ast.Expr{ast.NewIdent("params")},
				},
			},
		}

		ifStmt := &ast.IfStmt{
			Init: dispatchCall,
			Cond: ast.NewIdent("handled"),
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ReturnStmt{
						Results: []ast.Expr{ast.NewIdent("err")},
					},
				},
			},
		}

		fn.Body.List = append([]ast.Stmt{ifStmt}, fn.Body.List...)
		found = true
		return false
	})

	if alreadyExists {
		return nil
	}

	if !found {
		return fmt.Errorf("could not find run function in %s", path)
	}

	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to open file for writing: %w", err)
	}
	defer out.Close()

	if err := format.Node(out, fset, f); err != nil {
		return fmt.Errorf("failed to write modified file: %w", err)
	}

	return nil
}

func addCustomizationCalls(path string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	found := false
	alreadyExists := false
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runInteractive" {
			return true
		}

		// Check if callbacks already exist
		for _, stmt := range fn.Body.List {
			exprStmt, ok := stmt.(*ast.ExprStmt)
			if !ok {
				continue
			}
			callExpr, ok := exprStmt.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			if ident, ok := callExpr.Fun.(*ast.Ident); ok {
				if ident.Name == settingsCallback || ident.Name == commandCallback {
					alreadyExists = true
					return false
				}
			}
		}

		// Find the index of "cmd := otelcol.NewCommand(params)"
		for i, stmt := range fn.Body.List {
			assignStmt, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}

			// Check if this is "cmd := ..."
			if len(assignStmt.Lhs) != 1 {
				continue
			}
			ident, ok := assignStmt.Lhs[0].(*ast.Ident)
			if !ok || ident.Name != "cmd" {
				continue
			}

			// Create comment
			commentPos := assignStmt.Pos() - 1
			comment := &ast.Comment{
				Slash: commentPos,
				Text:  "// Added by builder/mod to allow customization of the collector settings and cobra command.",
			}
			commentGroup := &ast.CommentGroup{
				List: []*ast.Comment{comment},
			}

			// Create: customizeSettings(params)
			settingsCallExpr := &ast.CallExpr{
				Fun:  ast.NewIdent(settingsCallback),
				Args: []ast.Expr{ast.NewIdent("&params")},
			}
			settingsCallExpr.Fun.(*ast.Ident).NamePos = commentPos + 1
			settingsCall := &ast.ExprStmt{X: settingsCallExpr}

			// Create: customizeCommand(cmd)
			commandCall := &ast.ExprStmt{
				X: &ast.CallExpr{
					Fun:  ast.NewIdent(commandCallback),
					Args: []ast.Expr{ast.NewIdent("&params"), ast.NewIdent("cmd")},
				},
			}

			// Add comment to file's comment list
			f.Comments = append(f.Comments, commentGroup)

			// Build new statement list:
			// ... statements before cmd assignment ...
			// customizeSettings(params)       <- insert
			// cmd := otelcol.NewCommand(params)
			// customizeCommand(cmd)           <- insert
			// ... statements after cmd assignment ...
			newList := make([]ast.Stmt, 0, len(fn.Body.List)+2)
			newList = append(newList, fn.Body.List[:i]...)
			newList = append(newList, settingsCall)
			newList = append(newList, fn.Body.List[i]) // cmd := ...
			newList = append(newList, commandCall)
			newList = append(newList, fn.Body.List[i+1:]...)
			fn.Body.List = newList

			found = true
			return false
		}
		return true
	})

	if alreadyExists {
		// Already modified, nothing to do
		return nil
	}

	if !found {
		return fmt.Errorf("could not find cmd assignment in runInteractive function in %s", path)
	}

	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to open file for writing: %w", err)
	}
	defer out.Close()

	if err := format.Node(out, fset, f); err != nil {
		return fmt.Errorf("failed to write modified file: %w", err)
	}

	return nil
}
