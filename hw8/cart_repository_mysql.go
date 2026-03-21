package main

import (
	"context"
	"database/sql"
	"fmt"
)

type MySQLCartRepository struct {
	db         *sql.DB
	poolConfig MySQLPoolConfig
}

func NewMySQLCartRepository(cfg Config) (*MySQLCartRepository, error) {
	db, err := OpenMySQLDB(cfg)
	if err != nil {
		return nil, err
	}

	repo := &MySQLCartRepository{
		db:         db,
		poolConfig: cfg.MySQLPool,
	}

	if err := repo.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *MySQLCartRepository) CreateCart(ctx context.Context, customerID int32) (*ShoppingCart, error) {
	result, err := r.db.ExecContext(
		ctx,
		`INSERT INTO shopping_carts (customer_id, status)
		 VALUES (?, ?)`,
		customerID,
		string(CartStatusActive),
	)
	if err != nil {
		return nil, fmt.Errorf("insert shopping cart: %w", err)
	}

	cartID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get shopping cart id: %w", err)
	}

	return &ShoppingCart{
		ShoppingCartID: cartID,
		CustomerID:     customerID,
		Status:         CartStatusActive,
		Items:          []ShoppingCartItem{},
	}, nil
}

func (r *MySQLCartRepository) GetCart(ctx context.Context, cartID int64) (*ShoppingCart, error) {
	cart := &ShoppingCart{}

	err := r.db.QueryRowContext(
		ctx,
		`SELECT cart_id, customer_id, status, created_at, updated_at
		 FROM shopping_carts
		 WHERE cart_id = ?`,
		cartID,
	).Scan(
		&cart.ShoppingCartID,
		&cart.CustomerID,
		&cart.Status,
		&cart.CreatedAt,
		&cart.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("get shopping cart: %w", err)
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT cart_id, product_id, quantity, created_at, updated_at
		 FROM shopping_cart_items
		 WHERE cart_id = ?
		 ORDER BY product_id`,
		cartID,
	)
	if err != nil {
		return nil, fmt.Errorf("get shopping cart items: %w", err)
	}
	defer rows.Close()

	cart.Items = []ShoppingCartItem{}
	for rows.Next() {
		var item ShoppingCartItem
		if err := rows.Scan(
			&item.ShoppingCartID,
			&item.ProductID,
			&item.Quantity,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan shopping cart item: %w", err)
		}

		cart.Items = append(cart.Items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shopping cart items: %w", err)
	}

	return cart, nil
}

func (r *MySQLCartRepository) AddItem(ctx context.Context, cartID int64, productID int32, quantity int32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var status ShoppingCartStatus
	err = tx.QueryRowContext(
		ctx,
		`SELECT status
		 FROM shopping_carts
		 WHERE cart_id = ?
		 FOR UPDATE`,
		cartID,
	).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}

		return fmt.Errorf("lock shopping cart: %w", err)
	}

	if status != CartStatusActive {
		return ErrCartClosed
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO shopping_cart_items (cart_id, product_id, quantity)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		     quantity = quantity + VALUES(quantity),
		     updated_at = CURRENT_TIMESTAMP`,
		cartID,
		productID,
		quantity,
	)
	if err != nil {
		return fmt.Errorf("upsert shopping cart item: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE shopping_carts
		 SET updated_at = CURRENT_TIMESTAMP
		 WHERE cart_id = ?`,
		cartID,
	)
	if err != nil {
		return fmt.Errorf("update shopping cart timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	committed = true
	return nil
}

func (r *MySQLCartRepository) Close() error {
	if r.db == nil {
		return nil
	}

	return r.db.Close()
}

func (r *MySQLCartRepository) ensureSchema(ctx context.Context) error {
	cartsTableQuery := `
CREATE TABLE IF NOT EXISTS shopping_carts (
    cart_id BIGINT NOT NULL AUTO_INCREMENT,
    customer_id INT NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (cart_id),
    INDEX idx_customer_updated_at (customer_id, updated_at)
) ENGINE=InnoDB;
`

	if _, err := r.db.ExecContext(ctx, cartsTableQuery); err != nil {
		return fmt.Errorf("create shopping_carts table: %w", err)
	}

	itemsTableQuery := `
CREATE TABLE IF NOT EXISTS shopping_cart_items (
    cart_id BIGINT NOT NULL,
    product_id INT NOT NULL,
    quantity INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (cart_id, product_id),
    CONSTRAINT fk_cart_items_cart
        FOREIGN KEY (cart_id) REFERENCES shopping_carts(cart_id)
        ON DELETE CASCADE
) ENGINE=InnoDB;
`

	if _, err := r.db.ExecContext(ctx, itemsTableQuery); err != nil {
		return fmt.Errorf("create shopping_cart_items table: %w", err)
	}

	return nil
}
