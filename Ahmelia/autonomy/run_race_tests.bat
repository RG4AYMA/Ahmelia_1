@echo off
setlocal EnableExtensions

echo ==========================================
echo        AHMELIA - RACE DETECTOR
echo ==========================================
echo.

REM ==========================================
REM UBICACION DEL PROYECTO
REM Este BAT esta dentro de autonomy\
REM ==========================================

cd /d "%~dp0\.."

echo Directorio del proyecto:
cd
echo.

REM ==========================================
REM CONFIGURACION CGO + GCC
REM ==========================================

set "PATH=C:\msys64\ucrt64\bin;%PATH%"
set "CC=C:\msys64\ucrt64\bin\gcc.exe"
set "CGO_ENABLED=1"

REM ==========================================
REM VERIFICAR GCC
REM ==========================================

echo [1/5] Verificando GCC...

gcc --version

if errorlevel 1 (
    echo.
    echo ERROR: No se encontro GCC.
    echo.
    echo Verifica que exista:
    echo C:\msys64\ucrt64\bin\gcc.exe
    echo.
    pause
    exit /b 1
)

REM ==========================================
REM VERIFICAR CGO
REM ==========================================

echo.
echo [2/5] Verificando CGO...

go env CGO_ENABLED

if not "%CGO_ENABLED%"=="1" (
    echo.
    echo ERROR: CGO no esta habilitado.
    echo.
    pause
    exit /b 1
)

REM ==========================================
REM VERIFICAR GO MODULE
REM ==========================================

echo.
echo [3/5] Verificando modulo Go...

if not exist "go.mod" (
    echo.
    echo ERROR: No se encontro go.mod.
    echo.
    echo Directorio actual:
    cd
    echo.
    pause
    exit /b 1
)

go env GOMOD

REM ==========================================
REM RACE TEST - AUTONOMY
REM ==========================================

echo.
echo [4/5] Ejecutando tests de autonomy con race detector...
echo.

go test -race ./autonomy/...

if errorlevel 1 (
    echo.
    echo ==========================================
    echo ERROR: Fallaron los tests de autonomy.
    echo ==========================================
    echo.
    pause
    exit /b 1
)

echo.
echo ==========================================
echo AUTONOMY PASO EL RACE DETECTOR
echo ==========================================

REM ==========================================
REM RACE TEST - PROYECTO COMPLETO
REM ==========================================

echo.
echo [5/5] Ejecutando TODOS los tests con race detector...
echo.

go test -race ./...

if errorlevel 1 (
    echo.
    echo ==========================================
    echo ERROR: Fallaron los tests globales.
    echo ==========================================
    echo ==========================================
    echo.
    pause
    exit /b 1
)

echo.
echo ==========================================
echo.
echo       TODOS LOS TESTS PASARON
echo       RACE DETECTOR: OK
echo.
echo       AHMELIA B72-B77 VERIFICADO
echo.
echo ==========================================
echo.

pause
endlocal