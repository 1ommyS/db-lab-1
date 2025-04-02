package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

// Инициализация соединения с БД
func initDB() error {
	connStr := "host=localhost port=9432 user=dblab password=password dbname=dblab sslmode=disable"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	return db.Ping()
}

// Демонстрация неповторяемого чтения (Non-repeatable Read) в READ COMMITTED
func demoNonRepeatableRead(id int) {
	fmt.Printf("[NRR #%d] Начало демонстрации неповторяемого чтения\n", id)
	var wg sync.WaitGroup
	ch := make(chan struct{})
	wg.Add(2)

	// Транзакция A
	go func() {
		defer wg.Done()
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			log.Printf("[NRR #%d] TxA Begin: %v", id, err)
			return
		}
		var amount1 float64
		err = tx.QueryRowContext(ctx, "SELECT amount FROM payment WHERE payment_id = 1").Scan(&amount1)
		if err != nil {
			log.Printf("[NRR #%d] TxA первый SELECT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[NRR #%d] TxA первый SELECT: amount = %.2f", id, amount1)

		// Сигнал TxB начать обновление
		ch <- struct{}{}

		time.Sleep(2 * time.Second)
		var amount2 float64
		err = tx.QueryRowContext(ctx, "SELECT amount FROM payment WHERE payment_id = 1").Scan(&amount2)
		if err != nil {
			log.Printf("[NRR #%d] TxA второй SELECT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[NRR #%d] TxA второй SELECT: amount = %.2f", id, amount2)
		if err := tx.Commit(); err != nil {
			log.Printf("[NRR #%d] TxA Commit: %v", id, err)
		}
	}()

	// Транзакция B (обновление)
	go func() {
		defer wg.Done()
		<-ch
		time.Sleep(500 * time.Millisecond)
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			log.Printf("[NRR #%d] TxB Begin: %v", id, err)
			return
		}
		_, err = tx.ExecContext(ctx, "UPDATE payment SET amount = amount + 50, updated_at = now() WHERE payment_id = 1")
		if err != nil {
			log.Printf("[NRR #%d] TxB Update: %v", id, err)
			tx.Rollback()
			return
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[NRR #%d] TxB Commit: %v", id, err)
		}
		log.Printf("[NRR #%d] TxB обновил payment_id = 1", id)
	}()

	wg.Wait()
	fmt.Printf("[NRR #%d] Завершена демонстрация неповторяемого чтения\n", id)
}

// Демонстрация повторяемого чтения (Repeatable Read) в REPEATABLE READ
func demoRepeatableRead(id int) {
	fmt.Printf("[RR #%d] Начало демонстрации повторяемого чтения\n", id)
	var wg sync.WaitGroup
	ch := make(chan struct{})
	wg.Add(2)

	go func() {
		defer wg.Done()
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		if err != nil {
			log.Printf("[RR #%d] TxA Begin: %v", id, err)
			return
		}
		var amount1 float64
		err = tx.QueryRowContext(ctx, "SELECT amount FROM payment WHERE payment_id = 1").Scan(&amount1)
		if err != nil {
			log.Printf("[RR #%d] TxA первый SELECT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[RR #%d] TxA первый SELECT: amount = %.2f", id, amount1)

		ch <- struct{}{}
		time.Sleep(2 * time.Second)
		var amount2 float64
		err = tx.QueryRowContext(ctx, "SELECT amount FROM payment WHERE payment_id = 1").Scan(&amount2)
		if err != nil {
			log.Printf("[RR #%d] TxA второй SELECT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[RR #%d] TxA второй SELECT: amount = %.2f", id, amount2)
		if err := tx.Commit(); err != nil {
			log.Printf("[RR #%d] TxA Commit: %v", id, err)
		}
	}()

	go func() {
		defer wg.Done()
		<-ch
		time.Sleep(500 * time.Millisecond)
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		if err != nil {
			log.Printf("[RR #%d] TxB Begin: %v", id, err)
			return
		}
		_, err = tx.ExecContext(ctx, "UPDATE payment SET amount = amount + 50, updated_at = now() WHERE payment_id = 1")
		if err != nil {
			log.Printf("[RR #%d] TxB Update: %v", id, err)
			tx.Rollback()
			return
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[RR #%d] TxB Commit: %v", id, err)
		}
		log.Printf("[RR #%d] TxB обновил payment_id = 1", id)
	}()
	wg.Wait()
	fmt.Printf("[RR #%d] Завершена демонстрация повторяемого чтения\n", id)
}

// Демонстрация фантомного чтения (Phantom Read) в READ COMMITTED
func demoPhantomRead(id int) {
	fmt.Printf("[PR #%d] Начало демонстрации фантомного чтения\n", id)
	var wg sync.WaitGroup
	ch := make(chan struct{})
	wg.Add(2)

	threshold := 500.0

	go func() {
		defer wg.Done()
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			log.Printf("[PR #%d] TxA Begin: %v", id, err)
			return
		}
		var count1 int
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment WHERE amount > $1", threshold).Scan(&count1)
		if err != nil {
			log.Printf("[PR #%d] TxA первый SELECT COUNT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[PR #%d] TxA первый SELECT COUNT: %d строк с amount > %.2f", id, count1, threshold)

		// Сигнал на вставку новой строки
		ch <- struct{}{}
		time.Sleep(2 * time.Second)
		var count2 int
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment WHERE amount > $1", threshold).Scan(&count2)
		if err != nil {
			log.Printf("[PR #%d] TxA второй SELECT COUNT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[PR #%d] TxA второй SELECT COUNT: %d строк с amount > %.2f", id, count2, threshold)
		if err := tx.Commit(); err != nil {
			log.Printf("[PR #%d] TxA Commit: %v", id, err)
		}
	}()

	go func() {
		defer wg.Done()
		<-ch
		time.Sleep(500 * time.Millisecond)
		// Вставка новой записи, удовлетворяющей условию (amount > threshold)
		ctx := context.Background()
		_, err := db.ExecContext(ctx, `
			INSERT INTO payment (
				booking_id, customer_id, amount, currency, payment_time, payment_method, status, transaction_id,
				confirmation_code, refunded_amount, refund_time, error_code, error_message, notes, created_at, updated_at
			) VALUES ($1, $2, $3, 'USD', now(), 'Credit Card', 'COMPLETED', $4, $5, 0, NULL, NULL, NULL, 'Phantom test', now(), now())
		`, 9999, 8888, threshold+100, fmt.Sprintf("tx_pr_%d", id), fmt.Sprintf("conf_pr_%d", id))
		if err != nil {
			log.Printf("[PR #%d] TxB INSERT: %v", id, err)
			return
		}
		log.Printf("[PR #%d] TxB вставил новую запись с amount = %.2f", id, threshold+100)
	}()
	wg.Wait()
	fmt.Printf("[PR #%d] Завершена демонстрация фантомного чтения\n", id)
}

// Демонстрация ошибок сериализации (Serialization Failure) в SERIALIZABLE
func demoSerializationFailure(id int) {
	fmt.Printf("[SF #%d] Начало демонстрации ошибок сериализации\n", id)
	var wg sync.WaitGroup
	wg.Add(2)
	barrier := make(chan struct{})

	// Транзакция A
	go func() {
		defer wg.Done()
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			log.Printf("[SF #%d] TxA Begin: %v", id, err)
			return
		}
		var amount float64
		err = tx.QueryRowContext(ctx, "SELECT amount FROM payment WHERE payment_id = 1").Scan(&amount)
		if err != nil {
			log.Printf("[SF #%d] TxA SELECT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[SF #%d] TxA прочитал amount = %.2f", id, amount)
		// Сигнал TxB, что чтение завершено
		barrier <- struct{}{}
		time.Sleep(2 * time.Second)
		_, err = tx.ExecContext(ctx, "UPDATE payment SET amount = amount + 20, updated_at = now() WHERE payment_id = 1")
		if err != nil {
			log.Printf("[SF #%d] TxA UPDATE: %v", id, err)
			tx.Rollback()
			return
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[SF #%d] TxA Commit: %v", id, err)
		} else {
			log.Printf("[SF #%d] TxA успешно закоммитилась", id)
		}
	}()

	// Транзакция B
	go func() {
		defer wg.Done()
		<-barrier
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			log.Printf("[SF #%d] TxB Begin: %v", id, err)
			return
		}
		var amount float64
		err = tx.QueryRowContext(ctx, "SELECT amount FROM payment WHERE payment_id = 1").Scan(&amount)
		if err != nil {
			log.Printf("[SF #%d] TxB SELECT: %v", id, err)
			tx.Rollback()
			return
		}
		log.Printf("[SF #%d] TxB прочитал amount = %.2f", id, amount)
		time.Sleep(1 * time.Second)
		_, err = tx.ExecContext(ctx, "UPDATE payment SET amount = amount + 30, updated_at = now() WHERE payment_id = 1")
		if err != nil {
			log.Printf("[SF #%d] TxB UPDATE: %v", id, err)
			tx.Rollback()
			return
		}
		if err := tx.Commit(); err != nil {
			log.Printf("[SF #%d] TxB Commit: %v", id, err)
		} else {
			log.Printf("[SF #%d] TxB успешно закоммитилась", id)
		}
	}()
	wg.Wait()
	fmt.Printf("[SF #%d] Завершена демонстрация ошибок сериализации\n", id)
}

func main() {
	if err := initDB(); err != nil {
		log.Fatalf("Ошибка инициализации БД: %v", err)
	}
	defer db.Close()

	var wg sync.WaitGroup
	// Планируем запустить:
	// 4 экземпляра demoNonRepeatableRead
	// 4 экземпляра demoRepeatableRead
	// 4 экземпляра demoPhantomRead
	// 3 экземпляра demoSerializationFailure
	total := 4
	wg.Add(total)

	// Запуск экземпляров demoNonRepeatableRead
	for i := 1; i <= 4; i++ {
		go func(id int) {
			defer wg.Done()
			demoNonRepeatableRead(id)
		}(i)
	}

	//// Запуск экземпляров demoRepeatableRead
	//for i := 1; i <= 4; i++ {
	//	go func(id int) {
	//		defer wg.Done()
	//		demoRepeatableRead(id)
	//	}(i)
	//}
	//
	//// Запуск экземпляров demoPhantomRead
	//for i := 1; i <= 4; i++ {
	//	go func(id int) {
	//		defer wg.Done()
	//		demoPhantomRead(id)
	//	}(i)
	//}

	//// Запуск экземпляров demoSerializationFailure
	//for i := 1; i <= 3; i++ {
	//	go func(id int) {
	//		defer wg.Done()
	//		demoSerializationFailure(id)
	//	}(i)
	//}

	wg.Wait()
	fmt.Println("Все демонстрационные горутины завершены.")
}
