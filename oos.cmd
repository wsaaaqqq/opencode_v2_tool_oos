@echo off
chcp 65001 >nul
wsl.exe -e /root/.local/bin/oos %*
