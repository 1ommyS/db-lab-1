import psycopg2
from psycopg2.extras import execute_values
from faker import Faker
import random
from datetime import timedelta, timezone

# Параметры подключения к БД
DB_HOST = 'localhost'
DB_PORT = 9432
DB_NAME = 'dblab'
DB_USER = 'dblab'
DB_PASSWORD = 'password'

# Допустимые статусы платежа
STATUSES = ['PENDING', 'COMPLETED', 'FAILED', 'REFUNDED']

# Методы оплаты
PAYMENT_METHODS = ['Credit Card', 'PayPal', 'Bank Transfer', 'Cash']

faker = Faker()

def generate_payment_data():
    booking_id = random.randint(1000, 9999)
    customer_id = random.randint(1, 10000)
    amount = round(random.uniform(10, 1000), 2)
    # Генерируем время платежа в этом году с указанием часового пояса UTC
    payment_time = faker.date_time_this_year(before_now=True, after_now=False, tzinfo=timezone.utc)
    payment_method = random.choice(PAYMENT_METHODS)
    status = random.choice(STATUSES)
    transaction_id = faker.uuid4()
    confirmation_code = faker.bothify(text='??##??##')

    # Если статус "REFUNDED", генерируем случайную сумму возврата и время возврата (после payment_time)
    if status == 'REFUNDED':
        refunded_amount = round(random.uniform(0, amount), 2)
        refund_time = payment_time + timedelta(hours=random.randint(1, 72))
    else:
        refunded_amount = 0.00
        refund_time = None

    # Если статус "FAILED", генерируем код и сообщение об ошибке
    if status == 'FAILED':
        error_code = f"ERR{random.randint(100, 999)}"
        error_message = random.choice(['Insufficient funds', 'Card declined', 'Transaction timeout', 'Fraud suspected'])
    else:
        error_code = None
        error_message = None

    # Пример примечания (иногда None)
    notes = faker.sentence(nb_words=6) if random.choice([True, False]) else None

    # created_at и updated_at устанавливаем равными времени платежа
    created_at = payment_time
    updated_at = payment_time

    # Генерируем случайный ISO-код валюты (например, USD, EUR, GBP и т.п.)
    currency = faker.currency_code()

    return (
        booking_id,
        customer_id,
        amount,
        currency,
        payment_time,
        payment_method,
        status,
        transaction_id,
        confirmation_code,
        refunded_amount,
        refund_time,
        error_code,
        error_message,
        notes,
        created_at,
        updated_at
    )

num_records = 4_000_000
batch_size = 500_000

query = """
INSERT INTO payment (
    booking_id,
    customer_id,
    amount,
    currency,
    payment_time,
    payment_method,
    status,
    transaction_id,
    confirmation_code,
    refunded_amount,
    refund_time,
    error_code,
    error_message,
    notes,
    created_at,
    updated_at
) VALUES %s;
"""

try:
    conn = psycopg2.connect(
        host=DB_HOST,
        port=DB_PORT,
        dbname=DB_NAME,
        user=DB_USER,
        password=DB_PASSWORD
    )
    cur = conn.cursor()

    # Вставляем данные пакетами
    for i in range(0, num_records, batch_size):
        current_batch_size = min(batch_size, num_records - i)
        data_batch = [generate_payment_data() for _ in range(current_batch_size)]
        execute_values(cur, query, data_batch)
        conn.commit()
        print(f"Вставлен пакет {(i // batch_size) + 1} ({current_batch_size} записей)")

    print(f"Успешно вставлено {num_records} записей.")
except Exception as e:
    print("Ошибка при вставке данных:", e)
    conn.rollback()
finally:
    cur.close()
    conn.close()