#!/bin/bash

# Читаем JSON с параметрами из stdin
input=$(cat)

# Извлекаем числа a и b из JSON
a=$(echo "$input" | grep -o '"a":[^,}]*' | cut -d':' -f2 | tr -d ' ')
b=$(echo "$input" | grep -o '"b":[^,}]*' | cut -d':' -f2 | tr -d ' ')

# Вычисляем сумму
result=$(echo "$a + $b" | bc)

# Выводим результат в JSON формате
echo "{\"result\": $result, \"operation\": \"$a + $b = $result\"}"