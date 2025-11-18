ALTER TABLE movies
    ADD CONSTRAINT uq_movies_title_release_genre UNIQUE (title, release_date, genre);
