<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('idle_shutdown_settings', function (Blueprint $table) {
            $table->increments('id');
            $table->unsignedInteger('server_id')->unique();
            $table->boolean('enabled')->default(false);
            $table->unsignedInteger('idle_minutes')->nullable();
            $table->unsignedInteger('grace_minutes')->nullable();

            // Runtime state, managed by the checker.
            $table->timestamp('running_since')->nullable();
            $table->timestamp('empty_since')->nullable();
            $table->timestamp('last_stopped_at')->nullable();
            $table->timestamps();

            $table->foreign('server_id')->references('id')->on('servers')->cascadeOnDelete();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('idle_shutdown_settings');
    }
};
