@echo off
REM Generate go.sum file for Docker build caching
echo Generating go.sum file...
go mod download
go mod tidy
echo go.sum file generated successfully!
pause