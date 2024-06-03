@echo off
setlocal

REM Variables
set LOCAL_HOST="localhost"
set LOCAL_DB="......"
set LOCAL_USER="postgres"
set TABLE_NAME="......."
set DUMP_FILE="Table_dump.sql"

set RDS_HOST="....."
set RDS_DB="......"
set RDS_USER="postgres"

REM Dump the local table
pg_dump -h %LOCAL_HOST% -U %LOCAL_USER% -d %LOCAL_DB% -t %TABLE_NAME% -f %DUMP_FILE%
if errorlevel 1 (
    echo Failed to dump the local table
    exit /b 1
)

REM Import the dump into RDS
psql -h %RDS_HOST% -U %RDS_USER% -d %RDS_DB% -f %DUMP_FILE%
if errorlevel 1 (
    echo Failed to import the dump into RDS
    exit /b 1
)

REM Cleanup
del %DUMP_FILE%

echo Migration completed!
endlocal
pause